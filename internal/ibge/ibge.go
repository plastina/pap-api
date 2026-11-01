// Package ibge carrega a Tabela de Medidas Referidas para os Alimentos
// Consumidos no Brasil (IBGE, POF 2008-2009) e casa seus alimentos com os
// nomes da TACO.
//
// O CSV embutido e gerado por scripts/ibge_xls_to_csv.py a partir do arquivo
// oficial do IBGE (ver o cabecalho do script).
package ibge

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"saas-nutri/internal/client"
)

// Source identifica a origem das medidas gravadas na HouseholdMeasures.
const Source = "IBGE POF 2008-2009"

//go:embed medidas_pof2008.csv
var rawCSV []byte

type Measure struct {
	Name  string
	Grams float64
}

// Entry e um par alimento + preparo da tabela do IBGE com suas medidas.
type Entry struct {
	FoodCode int
	Food     string
	PrepCode int
	Prep     string
	Measures []Measure

	prep string // preparo canonico; "" = NAO SE APLICA
	// main: grupos de palavras que precisam aparecer no nome TACO (um grupo
	// com mais de uma palavra vem de "X OU Y"); paren: palavras entre
	// parenteses, que sao exemplos e so contam como bonus.
	main  [][]string
	paren []string
}

type Table struct {
	Entries []*Entry
	byCode  map[[2]int]*Entry
	// df: em quantas entradas cada palavra principal aparece; palavras raras
	// pesam mais no placar ("FIGADO BOVINO" vence "CARNE BOVINA").
	df map[string]int
}

// unitMeasures sao unidades de peso/volume, nao medidas caseiras: a medida
// base (grama/mililitro) ja cobre esses casos.
var unitMeasures = map[string]bool{"GRAMA": true, "QUILO": true, "MILILITRO": true, "LITRO": true}

// Load le a tabela embutida.
func Load() (*Table, error) {
	return parse(bytes.NewReader(rawCSV))
}

func parse(src io.Reader) (*Table, error) {
	r := csv.NewReader(src)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler cabecalho do CSV do IBGE: %w", err)
	}
	if strings.Join(header, ",") != "food_code,food,prep_code,prep,measure,grams" {
		return nil, fmt.Errorf("cabecalho inesperado no CSV do IBGE: %v", header)
	}

	t := &Table{byCode: map[[2]int]*Entry{}}
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("linha %d do CSV do IBGE: %w", line, err)
		}
		foodCode, err1 := strconv.Atoi(rec[0])
		prepCode, err2 := strconv.Atoi(rec[2])
		grams, err3 := strconv.ParseFloat(rec[5], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("linha %d do CSV do IBGE com numero invalido: %v", line, rec)
		}
		key := [2]int{foodCode, prepCode}
		e := t.byCode[key]
		if e == nil {
			e = newEntry(foodCode, rec[1], prepCode, rec[3])
			t.byCode[key] = e
			t.Entries = append(t.Entries, e)
		}
		if unitMeasures[rec[4]] || grams <= 0 {
			continue
		}
		name := MeasureName(rec[4])
		if !e.hasMeasure(name) {
			e.Measures = append(e.Measures, Measure{Name: name, Grams: grams})
		}
	}
	t.df = map[string]int{}
	for _, e := range t.Entries {
		for _, group := range e.main {
			for _, w := range group {
				t.df[w]++
			}
		}
	}
	return t, nil
}

// Entry devolve o par alimento + preparo pelos codigos do IBGE.
func (t *Table) Entry(foodCode, prepCode int) *Entry {
	return t.byCode[[2]int{foodCode, prepCode}]
}

// comOuSem casa "COM OU SEM SAL": o complemento e opcional, nao alternativa
// a palavra anterior ("MANTEIGA COM OU SEM SAL" nao e "manteiga ou sal").
var comOuSem = regexp.MustCompile(`\bCOM OU SEM \S+`)

func newEntry(foodCode int, food string, prepCode int, prep string) *Entry {
	e := &Entry{FoodCode: foodCode, Food: food, PrepCode: prepCode, Prep: prep, prep: ibgePrep(prep)}
	name := comOuSem.ReplaceAllString(food, "")
	mainPart, parenPart := name, ""
	if i := strings.Index(name, "("); i >= 0 {
		mainPart, parenPart = name[:i], name[i+1:]
		if j := strings.Index(parenPart, ")"); j >= 0 {
			mainPart += " " + parenPart[j+1:]
			parenPart = parenPart[:j]
		}
	}
	tokens := canonTokens(mainPart)
	for i := 0; i < len(tokens); i++ {
		if i+2 < len(tokens) && tokens[i+1] == "ou" {
			e.main = append(e.main, []string{tokens[i], tokens[i+2]})
			i += 2
			continue
		}
		if tokens[i] != "ou" {
			e.main = append(e.main, []string{tokens[i]})
		}
	}
	e.paren = canonTokens(parenPart)
	return e
}

func (e *Entry) hasMeasure(name string) bool {
	for _, m := range e.Measures {
		if m.Name == name {
			return true
		}
	}
	return false
}

// accents devolve a acentuacao que a tabela do IBGE (toda em maiusculas sem
// acento) perdeu, para os nomes de medida exibidos ao usuario.
var accents = map[string]string{
	"xicara": "xícara", "pedaco": "pedaço", "porcao": "porção", "cafe": "café",
	"cha": "chá", "file": "filé", "requeijao": "requeijão", "medio": "médio",
	"taca": "taça", "pescoco": "pescoço", "maco": "maço", "sache": "sachê",
	"pao": "pão", "mao": "mão",
}

// MeasureName converte o nome do IBGE ("XICARA DE CHA") para exibicao
// ("xícara de chá").
func MeasureName(raw string) string {
	raw = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), " (n. e.)", "")
	words := strings.Fields(raw)
	for i, w := range words {
		if a, ok := accents[w]; ok {
			words[i] = a
		}
	}
	s := strings.Join(words, " ")
	s = strings.ReplaceAll(s, " ml)", " mL)")
	return strings.ReplaceAll(s, " l)", " L)")
}

// prepWords leva as formas do preparo (no nome TACO ou do IBGE) ao preparo
// canonico.
var prepWords = map[string]string{
	"cozido": "cozido", "cozida": "cozido", "crozido": "cozido",
	"cru": "cru", "crua": "cru",
	"frito": "frito", "frita": "frito",
	"assado": "assado", "assada": "assado",
	"grelhado": "grelhado", "grelhada": "grelhado",
	"refogado": "refogado", "refogada": "refogado",
	"ensopado": "ensopado", "ensopada": "ensopado",
	"empanado": "empanado", "empanada": "empanado", "milanesa": "empanado",
}

func ibgePrep(prep string) string {
	if prep == "NAO SE APLICA" {
		return ""
	}
	tokens := client.NormalizeTokens(prep)
	if len(tokens) > 0 {
		if p, ok := prepWords[tokens[0]]; ok {
			return p
		}
	}
	// molho, sopa, vinagrete etc.: preparos que a TACO nao descreve
	return "outro:" + prep
}

var stopwords = map[string]bool{
	"de": true, "da": true, "do": true, "das": true, "dos": true, "com": true,
	"sem": true, "e": true, "em": true, "a": true, "o": true, "ao": true,
	"na": true, "no": true, "tipo": true, "etc": true, "nao": true,
	"especificado": true, "especificada": true, "ne": true, "varejo": true,
	"qualquer": true,
}

var synonyms = map[string]string{
	"muzarella": "mucarela", "muzarela": "mucarela", "mozarela": "mucarela",
	"mozzarella": "mucarela", "mussarela": "mucarela", "muzzarella": "mucarela",
	"bovina": "bovino", "suina": "suino", "caprina": "caprino", "porco": "suino",
}

// tacoAliases acrescentam ao nome TACO a palavra que o IBGE usa para o mesmo
// alimento: o pao frances e "PAO DE SAL" na POF, coxa e sobrecoxa estao em
// "FRANGO EM PEDACOS", enlatado e "EM CONSERVA" e as especies de peixe da TACO
// caem em "PEIXE NAO ESPECIFICADO". Um alias da primeira palavra tambem vale
// como primeira palavra.
var tacoAliases = map[string][]string{
	"frances": {"sal"}, "coxa": {"pedaco"}, "sobrecoxa": {"pedaco"},
	"enlatado": {"conserva"}, "enlatada": {"conserva"},
	"abadejo": {"peixe"}, "pescada": {"peixe"}, "pescadinha": {"peixe"},
	"merluza": {"peixe"}, "pintado": {"peixe"}, "corvina": {"peixe"},
	"cacao": {"peixe"}, "corimba": {"peixe"}, "corimbata": {"peixe"},
	"dourada": {"peixe"}, "tucunare": {"peixe"}, "salmao": {"peixe"},
	"sardinha": {"peixe"}, "tilapia": {"peixe"}, "atum": {"peixe"},
}

// distinctive sao palavras que mudam a medida caseira do alimento: um nome
// TACO com uma delas so casa com entrada do IBGE que tambem a tenha (a clara
// nao herda a "unidade" do ovo inteiro, o parmesao ralado nao herda a fatia).
var distinctive = map[string]bool{
	"clara": true, "gema": true, "petit": true, "parmesao": true,
	"ralado": true, "chip": true, "po": true, "suco": true, "polpa": true,
	"extrato": true, "pure": true, "tablete": true, "almondega": true,
	"farofa": true, "calda": true, "pasta": true, "barra": true, "cascao": true,
}

// canonTokens normaliza um nome para comparacao: minusculas sem acento, sem
// palavras vazias, plural reduzido e sinonimos unificados.
func canonTokens(s string) []string {
	var out []string
	for _, tk := range client.NormalizeTokens(s) {
		if stopwords[tk] {
			continue
		}
		if p, ok := prepWords[tk]; ok {
			tk = p
		}
		tk = singular(tk)
		if syn, ok := synonyms[tk]; ok {
			tk = syn
		}
		out = append(out, tk)
	}
	return out
}

func singular(tk string) string {
	switch {
	case len(tk) > 4 && strings.HasSuffix(tk, "oes"):
		return tk[:len(tk)-3] + "ao"
	case tk == "paes":
		return "pao"
	case len(tk) > 3 && strings.HasSuffix(tk, "s") && !strings.HasSuffix(tk, "ss") && !strings.HasSuffix(tk, "es"):
		return tk[:len(tk)-1]
	}
	return tk
}

// rawNeedsExplicit sao alimentos cujas medidas "NAO SE APLICA" no IBGE ja se
// referem ao alimento pronto (ex.: arroz cozido); a versao crua da TACO so
// casa com uma entrada explicitamente CRU(A).
var rawNeedsExplicit = map[string]bool{
	"arroz": true, "feijao": true, "macarrao": true, "lentilha": true,
	"ervilha": true, "grao": true, "soja": true,
	"carne": true, "frango": true, "peixe": true, "file": true,
}

// Match encontra a entrada do IBGE para um nome da TACO. Casa quando todas as
// palavras principais do IBGE aparecem no nome TACO (o IBGE e mais generico:
// "CARNE BOVINA" serve para "Carne, bovina, acem, moido, cozido"), a primeira
// palavra da TACO esta no nome do IBGE e o preparo e compativel. Entre as
// candidatas vence a mais especifica.
func (t *Table) Match(tacoName string) (*Entry, float64) {
	tokens := canonTokens(tacoName)
	if len(tokens) == 0 {
		return nil, 0
	}
	// primary: palavras antes de "com"; o que vem depois sao ingredientes
	// ("Pipoca, com oleo de soja" nao e oleo de soja).
	raw := client.NormalizeTokens(tacoName)
	for i, tk := range raw {
		if tk == "com" {
			raw = raw[:i]
			break
		}
	}
	primary := map[string]bool{}
	for _, tk := range canonTokens(strings.Join(raw, " ")) {
		primary[tk] = true
		for _, a := range tacoAliases[tk] {
			primary[a] = true
		}
	}
	have := map[string]bool{}
	var needed []string
	prep := ""
	for _, tk := range tokens {
		have[tk] = true
		for _, a := range tacoAliases[tk] {
			have[a] = true
		}
		if _, ok := prepWords[tk]; ok {
			prep = tk
		}
		if distinctive[tk] {
			needed = append(needed, tk)
		}
	}
	heads := append([]string{tokens[0]}, tacoAliases[tokens[0]]...)

	var best *Entry
	bestScore := 0.0
	for _, e := range t.Entries {
		if len(e.Measures) == 0 || len(e.main) == 0 {
			continue
		}
		prepScore, ok := prepCompat(prep, e.prep, heads)
		if !ok {
			continue
		}
		score, ok := t.nameScore(e, have, primary, heads)
		if !ok || !e.hasAll(needed) {
			continue
		}
		score += prepScore
		for _, w := range e.paren {
			if have[w] {
				score += 0.3
			}
		}
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	return best, bestScore
}

// nameScore exige que cada grupo de palavras principais do IBGE apareca no
// nome TACO e que a primeira palavra da TACO esteja entre elas. Nomes do IBGE
// com duas ou mais palavras que nao sejam preparo dispensam essa ultima
// condicao ("FIGADO BOVINO" para "Carne, bovina, figado"), desde que nenhuma
// venha de depois do "com" da TACO. Cada palavra vale 1 + 1/df.
func (t *Table) nameScore(e *Entry, have, primary map[string]bool, heads []string) (float64, bool) {
	score := 0.0
	headFound := false
	allPrimary := true
	nonPrep := 0
	for _, group := range e.main {
		found := ""
		for _, w := range group {
			if have[w] {
				found = w
			}
			if containsWord(heads, w) {
				headFound = true
			}
		}
		if found == "" {
			return 0, false
		}
		if !primary[found] {
			allPrimary = false
		}
		if _, isPrep := prepWords[found]; !isPrep {
			nonPrep++
		}
		score += 1 + 1/float64(t.df[found])
	}
	if !headFound && (nonPrep < 2 || !allPrimary) {
		return 0, false
	}
	return score, true
}

func containsWord(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

func (e *Entry) hasAll(words []string) bool {
	for _, w := range words {
		ok := false
		for _, group := range e.main {
			for _, g := range group {
				if g == w {
					ok = true
				}
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// prepCompat diz se o preparo do IBGE serve para o preparo da TACO e quanto
// isso soma ao placar (preparo identico vale mais que "NAO SE APLICA").
func prepCompat(taco, ibge string, heads []string) (float64, bool) {
	switch {
	case ibge == taco:
		return 0.5, true
	case ibge != "":
		return 0, false
	case taco == "cru" && anyRawNeedsExplicit(heads):
		return 0, false
	}
	return 0, true
}

func anyRawNeedsExplicit(heads []string) bool {
	for _, h := range heads {
		if rawNeedsExplicit[h] {
			return true
		}
	}
	return false
}

package client

// Medidas caseiras por tipo de alimento. A tabela HouseholdMeasures do banco
// so tem um trio generico igual para todos os alimentos (colher de sopa 15 g,
// unidade 50 g, xicara 100 g), que nao serve para queijo, pao, ovo etc.
// Os pesos abaixo sao referencias aproximadas de medidas caseiras usuais no
// Brasil e devem ser revisados por nutricionista.

type measureRule struct {
	// first: primeira palavra normalizada do nome (ex.: "queijo").
	first []string
	// any: se preenchido, alguma dessas palavras precisa aparecer no nome.
	any      []string
	measures []MeasureItem
}

func hm(name string, grams float64) MeasureItem {
	return MeasureItem{MeasureName: name, DisplayName: name, GramEquivalent: grams}
}

var (
	queijoFatiado = []MeasureItem{
		hm("fatia fina", 15), hm("fatia média", 20), hm("fatia grossa", 30),
	}
	mucarelaWords = []string{"mozarela", "mozzarella", "mussarela", "mucarela", "muzzarella"}
)

// measureRules e avaliada em ordem; vale a primeira regra que casar.
var measureRules = []measureRule{
	{first: []string{"queijo"}, any: []string{"parmesao", "ralado"}, measures: []MeasureItem{
		hm("colher de sopa (ralado)", 5), hm("fatia média", 15),
	}},
	{first: []string{"queijo", "requeijao"}, any: []string{"requeijao", "cremoso"}, measures: []MeasureItem{
		hm("colher de chá", 10), hm("colher de sopa", 30),
	}},
	{first: []string{"queijo", "ricota"}, any: []string{"ricota"}, measures: []MeasureItem{
		hm("colher de sopa", 20), hm("fatia média", 30),
	}},
	{first: []string{"queijo"}, any: []string{"petit"}, measures: []MeasureItem{
		hm("pote", 45),
	}},
	{first: []string{"queijo"}, any: append([]string{"prato", "lanche", "cheddar"}, mucarelaWords...), measures: queijoFatiado},
	{first: mucarelaWords, measures: queijoFatiado},
	{first: []string{"queijo"}, measures: []MeasureItem{
		hm("fatia média", 30), hm("fatia grossa", 40),
	}},
	{first: []string{"ovo"}, any: []string{"codorna"}, measures: []MeasureItem{hm("unidade", 10)}},
	{first: []string{"ovo"}, any: []string{"clara"}, measures: []MeasureItem{hm("clara de 1 ovo", 30)}},
	{first: []string{"ovo"}, any: []string{"gema"}, measures: []MeasureItem{hm("gema de 1 ovo", 18)}},
	{first: []string{"ovo"}, measures: []MeasureItem{hm("unidade", 50)}},
	{first: []string{"pao"}, any: []string{"forma"}, measures: []MeasureItem{hm("fatia", 25)}},
	{first: []string{"pao"}, any: []string{"frances"}, measures: []MeasureItem{hm("unidade", 50)}},
	{first: []string{"pao"}, any: []string{"queijo"}, measures: []MeasureItem{hm("unidade média", 20)}},
	{first: []string{"pao"}, measures: []MeasureItem{hm("fatia", 30), hm("unidade", 50)}},
	{first: []string{"arroz"}, any: []string{"cozido", "carreteiro"}, measures: []MeasureItem{
		hm("colher de sopa cheia", 25), hm("escumadeira", 90),
	}},
	{first: []string{"feijao"}, any: []string{"cozido", "tropeiro"}, measures: []MeasureItem{
		hm("colher de sopa cheia", 20), hm("concha média", 86),
	}},
	{first: []string{"manteiga", "margarina"}, measures: []MeasureItem{
		hm("ponta de faca", 4), hm("colher de chá", 5), hm("colher de sopa", 15),
	}},
}

// measuresForName devolve as medidas caseiras do tipo de alimento, ou nil se
// nenhuma regra se aplicar.
func measuresForName(name string) []MeasureItem {
	tokens := NormalizeTokens(name)
	if len(tokens) == 0 {
		return nil
	}
	for _, rule := range measureRules {
		if !containsWord(rule.first, tokens[0]) {
			continue
		}
		if len(rule.any) > 0 && !anyWord(rule.any, tokens) {
			continue
		}
		return rule.measures
	}
	return nil
}

// isGenericPlaceholder identifica o trio generico do banco, descartado quando
// o alimento tem medidas proprias do seu tipo.
func isGenericPlaceholder(item MeasureItem) bool {
	if item.MeasureSource != "" {
		return false
	}
	switch item.MeasureName {
	case "colher de sopa":
		return item.GramEquivalent == 15
	case "unidade":
		return item.GramEquivalent == 50
	case "xícara":
		return item.GramEquivalent == 100
	}
	return false
}

// hasSourcedMeasure informa se ha medida de fonte oficial (ex.: IBGE).
func hasSourcedMeasure(items []MeasureItem) bool {
	for _, m := range items {
		if m.MeasureSource != "" {
			return true
		}
	}
	return false
}

func containsWord(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

func anyWord(words, tokens []string) bool {
	for _, tk := range tokens {
		if containsWord(words, tk) {
			return true
		}
	}
	return false
}

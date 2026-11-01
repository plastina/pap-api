package client

import (
	"strings"
	"unicode"
)

// diacriticReplacer remove acentos comuns do portugues. A base TACO e gravada
// sem acentos, e a busca precisa casar termos com ou sem acentuacao.
var diacriticReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// NormalizeTokens quebra um nome de alimento nas palavras usadas pelo indice de
// busca. Aplica lowercase, remove acentos e trata qualquer caractere que nao
// seja letra ou digito como separador. Ex.: "Queijo, requeijão, cremoso" ->
// ["queijo", "requeijao", "cremoso"].
//
// E a unica fonte de normalizacao: usada tanto na escrita dos tokens quanto na
// busca, para as duas nao divergirem.
func NormalizeTokens(s string) []string {
	s = diacriticReplacer.Replace(strings.ToLower(s))
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// stopwords sao palavras de ligacao ignoradas na busca ("agua de coco" ->
// agua, coco). Os tokens do indice continuam sendo gravados com elas.
var stopwords = map[string]bool{
	"a": true, "o": true, "as": true, "os": true, "e": true,
	"de": true, "da": true, "do": true, "das": true, "dos": true,
	"com": true, "sem": true, "em": true, "na": true, "no": true,
	"para": true, "p": true,
}

// searchTokens reduz o termo de busca bruto aos tokens que o alimento precisa
// casar (todos, como prefixo de alguma palavra do nome). Remove stopwords,
// exceto se o termo for so stopwords. Termo vazio retorna nil.
func searchTokens(term string) []string {
	tokens := NormalizeTokens(term)
	out := make([]string, 0, len(tokens))
	for _, tk := range tokens {
		if !stopwords[tk] {
			out = append(out, tk)
		}
	}
	if len(out) == 0 {
		return tokens
	}
	return out
}

// queryToken escolhe o token consultado no indice: o mais longo, por ser em
// geral o mais seletivo, para o teto da Query nao cortar os alimentos certos.
func queryToken(tokens []string) string {
	best := ""
	for _, tk := range tokens {
		if len(tk) > len(best) {
			best = tk
		}
	}
	return best
}

// tokenPartition deriva a chave de particao de um token a partir da primeira
// letra, evitando uma particao unica (quente) no indice de busca. Tokens que
// nao comecam com a-z caem na particao "_".
func tokenPartition(token string) string {
	if token == "" {
		return "_"
	}
	first := []rune(token)[0]
	if first >= 'a' && first <= 'z' {
		return string(first)
	}
	return "_"
}

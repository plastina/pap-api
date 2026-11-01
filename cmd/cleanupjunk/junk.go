package main

import (
	"strings"

	"saas-nutri/internal/client"
)

// junkExact sao nomes (ja normalizados por client.NormalizeTokens) que so
// aparecem como cabecalho ou nota no PDF da TACO.
var junkExact = []string{
	"descricao dos alimentos",
	"l",
	"as analises estao sendo reavaliadas",
}

// junkPrefixes sao inicios de notas de rodape longas; o resto do texto varia.
var junkPrefixes = []string{
	"limites de quantificacao",
	"abreviacoes",
	"teores alcoolicos",
	"valores em branco nesta tabela",
	"valores correspondentes a somatoria",
	"valores retirados do livro fontes brasileiras de carotenoides",
}

// junkNameMatch diz se o nome casa a lista conhecida. A comparacao e por
// palavras inteiras: o prefixo "l" (exato) nao casa "Leite".
func junkNameMatch(name string) bool {
	tokens := client.NormalizeTokens(name)
	joined := strings.Join(tokens, " ")
	for _, e := range junkExact {
		if joined == e {
			return true
		}
	}
	for _, p := range junkPrefixes {
		if joined == p || strings.HasPrefix(joined, p+" ") {
			return true
		}
	}
	return false
}

func zeroNutrients(f client.TacoFoodItem) bool {
	return f.EnergyKcal == 0 && f.ProteinG == 0 && f.CarbohydrateG == 0 &&
		f.FatG == 0 && f.FiberG == 0
}

// isJunk e a regra de exclusao: linha TACO, nome na lista e nutrientes zerados.
func isJunk(f client.TacoFoodItem) bool {
	return f.DataSource == "TACO" && junkNameMatch(f.OriginalName) && zeroNutrients(f)
}

// classify separa as linhas-lixo (apagaveis) das suspeitas: nome na lista mas
// com algum nutriente, que ficam so no relatorio para revisao manual.
func classify(foods []client.TacoFoodItem) (junk, suspects []client.TacoFoodItem) {
	for _, f := range foods {
		switch {
		case isJunk(f):
			junk = append(junk, f)
		case f.DataSource == "TACO" && junkNameMatch(f.OriginalName):
			suspects = append(suspects, f)
		}
	}
	return junk, suspects
}

package client

const (
	// BaseMeasureGram e BaseMeasureMl sao os nomes da medida-base (1 unidade)
	// de cada alimento. Os valores nutricionais da base sao por 100 g; para
	// liquidos assume-se 1 mL ~ 1 g.
	BaseMeasureGram = "grama"
	BaseMeasureMl   = "mililitro"

	UnitGram = "g"
	UnitMl   = "ml"
)

// liquidFirstWords sao as primeiras palavras de nomes de alimentos liquidos.
// So a primeira palavra conta, para "Pao de leite" ou "Bolo de cerveja" nao
// serem tratados como liquidos.
var liquidFirstWords = map[string]bool{
	"agua": true, "suco": true, "leite": true, "refrigerante": true,
	"bebida": true, "cafe": true, "cha": true, "cerveja": true, "vinho": true,
	"cachaca": true, "aguardente": true, "whisky": true, "uisque": true,
	"vodka": true, "rum": true, "licor": true, "champanhe": true,
	"oleo": true, "azeite": true, "vinagre": true, "caldo": true,
	"vitamina": true, "isotonico": true, "energetico": true, "nectar": true,
	"limonada": true, "kefir": true, "kombucha": true,
}

// notLiquidWords descartam nomes cuja forma de preparo nao e liquida, mesmo
// comecando por uma palavra liquida (ex.: "Leite, em po", "Cafe, po").
var notLiquidWords = map[string]bool{
	"po": true, "condensado": true, "tablete": true, "cubo": true,
	"concentrado": true, "granulado": true, "solúvel": true, "soluvel": true,
	"gelatina": true, "bala": true, "doce": true, "sorvete": true,
}

// IsLiquid informa se o alimento e medido em mL em vez de gramas.
func IsLiquid(name string) bool {
	tokens := NormalizeTokens(name)
	if len(tokens) == 0 || !liquidFirstWords[tokens[0]] {
		return false
	}
	for _, tk := range tokens[1:] {
		if notLiquidWords[tk] {
			return false
		}
	}
	return true
}

// UnitForFood devolve "ml" para liquidos e "g" para os demais.
func UnitForFood(name string) string {
	if IsLiquid(name) {
		return UnitMl
	}
	return UnitGram
}

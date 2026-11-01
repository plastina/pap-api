package ibge

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		taco, food, prep string // food "" = sem casamento
	}{
		{"Queijo, mozarela", "QUEIJO MUZARELLA", "NAO SE APLICA"},
		{"Queijo, prato", "QUEIJO PRATO", "NAO SE APLICA"},
		{"Queijo, minas, frescal", "QUEIJO DE MINAS", "NAO SE APLICA"},
		{"Queijo, ricota", "QUEIJO RICOTA", "NAO SE APLICA"},
		{"Queijo, parmesão", "", ""},
		{"Queijo, petit suisse, morango", "", ""},
		{"Arroz, tipo 1, cozido", "ARROZ (POLIDO, PARBOILIZADO)", "NAO SE APLICA"},
		{"Arroz, integral, cozido", "ARROZ INTEGRAL", "NAO SE APLICA"},
		{"Arroz, tipo 1, cru", "", ""},
		{"Feijão, carioca, cozido", "FEIJAO (PRETO, MULATINHO, ROXO, ROSINHA, ETC)", "NAO SE APLICA"},
		{"Feijão, carioca, cru", "", ""},
		{"Ovo, de galinha, inteiro, cozido/10minutos", "OVO DE GALINHA", "CROZIDO(A)"},
		{"Ovo, de galinha, clara, cozida/10minutos", "", ""},
		{"Ovo, de codorna, inteiro, cru", "OVO DE CODORNA", "CRU(A)"},
		{"Pão, trigo, francês", "PAO DE SAL", "NAO SE APLICA"},
		{"Pão, de queijo, assado", "PAO DE QUEIJO", "NAO SE APLICA"},
		{"Frango, peito, sem pele, grelhado", "PEITO DE GALINHA OU FRANGO", "GRELHADO(A)/BRASA/CHURRASCO"},
		{"Carne, bovina, acém, moído, cozido", "CARNE BOVINA", "CROZIDO(A)"},
		{"Carne, bovina, fígado, grelhado", "FIGADO BOVINO", "GRELHADO(A)/BRASA/CHURRASCO"},
		{"Banana, prata, crua", "BANANA (OURO, PRATA, D´AGUA, DA TERRA, ETC)", "CRU(A)"},
		{"Leite, de vaca, desnatado, pó", "LEITE EM PO DESNATADO", "NAO SE APLICA"},
		{"Manteiga, com sal", "MANTEIGA COM OU SEM SAL", "NAO SE APLICA"},
		{"Batata, inglesa, frita, tipo chips, industrializada", "", ""},
		{"Macarrão, trigo, cru, com ovos", "MACARRAO", "CRU(A)"},
		{"Aveia, flocos, crua", "AVEIA EM FLOCOS", "NAO SE APLICA"},
		{"Salsicha", "SALSICHA NO VAREJO", "NAO SE APLICA"},
	}
	for _, tc := range cases {
		e, _ := tb.Match(tc.taco)
		got := ""
		gotPrep := ""
		if e != nil {
			got, gotPrep = e.Food, e.Prep
		}
		if got != tc.food || gotPrep != tc.prep {
			t.Errorf("Match(%q) = %q / %q, want %q / %q", tc.taco, got, gotPrep, tc.food, tc.prep)
		}
	}
}

func TestMucarelaSlice(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	e, _ := tb.Match("Queijo, mozarela")
	for _, m := range e.Measures {
		if m.Name == "fatia" {
			if m.Grams != 20 {
				t.Errorf("fatia de mucarela = %v g, want 20", m.Grams)
			}
			return
		}
	}
	t.Error("mucarela sem medida fatia")
}

func TestLoadDropsUnitsAndRestoresAccents(t *testing.T) {
	tb, err := parse(strings.NewReader("food_code,food,prep_code,prep,measure,grams\n" +
		"1,ARROZ,99,NAO SE APLICA,GRAMA,1.0\n" +
		"1,ARROZ,99,NAO SE APLICA,QUILO,1000.0\n" +
		"1,ARROZ,99,NAO SE APLICA,XICARA DE CHA,160.0\n" +
		"1,ARROZ,99,NAO SE APLICA,LATA (350 ML),350.0\n"))
	if err != nil {
		t.Fatal(err)
	}
	ms := tb.Entry(1, 99).Measures
	if len(ms) != 2 || ms[0].Name != "xícara de chá" || ms[1].Name != "lata (350 mL)" {
		t.Errorf("medidas = %+v", ms)
	}
}

func TestMatchReviewFixes(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ taco, food string }{
		{"Sal, grosso", ""},
		{"Manteiga, sem sal", "MANTEIGA COM OU SEM SAL"},
		{"Pipoca, com óleo de soja, sem sal", ""},
		{"Chocolate, ao leite, com castanha do Pará", "CHOCOLATE"},
		{"Goiaba, doce em pasta", ""},
		{"Banana, doce em barra", ""},
		{"Caju, suco concentrado, envasado", ""},
		{"Laranja, pêra, suco", "SUCO DE LARANJA"},
		{"Manga, polpa, congelada", ""},
		{"Tomate, extrato", ""},
		{"Caldo de carne, tablete", ""},
		{"Cuscuz, de milho, cozido com sal", "CUSCUZ"},
		{"Porco, lombo, assado", "LOMBO SUINO"},
		{"Pescada, filé, frito", "PEIXE NAO ESPECIFICADO (INTEIRO, EM POSTA, EM FILE, ETC)"},
		{"Frango, sobrecoxa, com pele, assada", "FRANGO EM PEDACOS"},
		{"Iogurte, sabor morango", "IOGURTE DE QUALQUER SABOR"},
		{"Milho, verde, enlatado, drenado", "MILHO VERDE EM CONSERVA"},
	}
	for _, tc := range cases {
		e, _ := tb.Match(tc.taco)
		got := ""
		if e != nil {
			got = e.Food
		}
		if got != tc.food {
			t.Errorf("Match(%q) = %q, want %q", tc.taco, got, tc.food)
		}
	}
}

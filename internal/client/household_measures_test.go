package client

import "testing"

func TestMeasuresForName(t *testing.T) {
	cases := []struct {
		name      string
		wantFirst string
		wantGrams float64
	}{
		{"Queijo, mozarela", "fatia fina", 15},
		{"Mussarela (Italac)", "fatia fina", 15},
		{"Muçarela Light (Queijos Scala e Verdemar)", "fatia fina", 15},
		{"Queijo, prato", "fatia fina", 15},
		{"Queijo, parmesão", "colher de sopa (ralado)", 5},
		{"Queijo, requeijão, cremoso", "colher de chá", 10},
		{"Requeijão", "colher de chá", 10},
		{"Queijo, ricota", "colher de sopa", 20},
		{"Queijo, minas, meia cura", "fatia média", 30},
		{"Ovo, de galinha, inteiro, cozido/10minutos", "unidade", 50},
		{"Ovo, de galinha, clara, cozida/10minutos", "clara de 1 ovo", 30},
		{"Ovo, de codorna, inteiro, cru", "unidade", 10},
		{"Pão, trigo, forma, integral", "fatia", 25},
		{"Pão, trigo, francês", "unidade", 50},
		{"Pão de queijo, assado", "unidade média", 20},
		{"Arroz, tipo 1, cozido", "colher de sopa cheia", 25},
		{"Feijão, carioca, cozido", "colher de sopa cheia", 20},
		{"Manteiga, com sal", "ponta de faca", 4},
	}
	for _, tc := range cases {
		got := measuresForName(tc.name)
		if len(got) == 0 {
			t.Errorf("measuresForName(%q) = nenhuma medida", tc.name)
			continue
		}
		if got[0].MeasureName != tc.wantFirst || got[0].GramEquivalent != tc.wantGrams {
			t.Errorf("measuresForName(%q)[0] = %s %vg, want %s %vg",
				tc.name, got[0].MeasureName, got[0].GramEquivalent, tc.wantFirst, tc.wantGrams)
		}
	}
}

func TestMeasuresForNameNoRule(t *testing.T) {
	for _, name := range []string{"Arroz, tipo 1, cru", "Macarrão, trigo, cru, com ovos", "Banana, prata, crua", ""} {
		if got := measuresForName(name); got != nil {
			t.Errorf("measuresForName(%q) = %v, want nil", name, got)
		}
	}
}

func TestIsGenericPlaceholder(t *testing.T) {
	if !isGenericPlaceholder(MeasureItem{MeasureName: "unidade", GramEquivalent: 50}) {
		t.Error("unidade 50 g deveria ser placeholder")
	}
	if isGenericPlaceholder(MeasureItem{MeasureName: "unidade", GramEquivalent: 10}) {
		t.Error("unidade 10 g nao e placeholder")
	}
}

func TestIsGenericPlaceholderIgnoresSourcedRows(t *testing.T) {
	// "colher de sopa" do IBGE com 15 g e medida real, nao placeholder
	m := MeasureItem{MeasureName: "colher de sopa", GramEquivalent: 15, MeasureSource: "IBGE POF 2008-2009"}
	if isGenericPlaceholder(m) {
		t.Error("medida com origem nao deveria ser placeholder")
	}
	if !hasSourcedMeasure([]MeasureItem{{}, m}) {
		t.Error("hasSourcedMeasure deveria achar a medida do IBGE")
	}
}

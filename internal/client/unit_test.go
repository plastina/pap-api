package client

import "testing"

func TestIsLiquid(t *testing.T) {
	cases := map[string]bool{
		"Água de coco, verde":        true,
		"Leite, de vaca, integral":   true,
		"Suco de laranja":            true,
		"Café, infusão 10%":          true,
		"Óleo, de soja":              true,
		"Leite, em pó, integral":     false,
		"Leite, condensado":          false,
		"Café, pó":                   false,
		"Pão, de leite":              false,
		"Arroz, integral, cozido":    false,
		"Caldo de carne, tablete":    false,
		"":                           false,
	}
	for name, want := range cases {
		if got := IsLiquid(name); got != want {
			t.Errorf("IsLiquid(%q) = %v, want %v", name, got, want)
		}
	}
}

package main

import (
	"testing"

	"saas-nutri/internal/client"
)

func TestJunkNameMatch(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Descrição dos alimentos", true},
		{"DESCRIÇÃO DOS ALIMENTOS", true},
		{"L", true},
		{"as análises estão sendo reavaliadas", true},
		{"Limites de Quantificação: 0,1 g para proteína ...", true},
		{"Abreviações: NA = não aplicável; Tr = traço", true},
		{"Teores alcoólicos (g/100g): cerveja 3,6 ...", true},
		{"Valores em branco nesta tabela: análises não solicitadas", true},
		{"Valores correspondentes à somatória dos isômeros", true},
		{"Valores retirados do livro Fontes brasileiras de carotenóides (2008)", true},

		// alimentos reais com palavras parecidas
		{"Leite, de vaca, integral", false},
		{"Limão, tahiti, cru", false},
		{"Lasanha, massa fresca, crua", false},
		{"Alimentos, descrição", false},
		{"Valores", false},
		{"Limites", false},
		{"Abacate, cru", false},
		{"", false},
	}
	for _, c := range cases {
		if got := junkNameMatch(c.name); got != c.want {
			t.Errorf("junkNameMatch(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassify(t *testing.T) {
	foods := []client.TacoFoodItem{
		{FoodID: "1", DataSource: "TACO", OriginalName: "Descrição dos alimentos"},
		{FoodID: "2", DataSource: "TACO", OriginalName: "Descrição dos alimentos"},
		{FoodID: "3", DataSource: "TACO", OriginalName: "L"},
		// nome de lixo mas com nutriente: so suspeito, nunca apagado
		{FoodID: "4", DataSource: "TACO", OriginalName: "L", EnergyKcal: 12},
		// mesmo nome vindo do OFF: fora do escopo
		{FoodID: "5", DataSource: "OFF", OriginalName: "Descrição dos alimentos"},
		// alimento real zerado (ex.: agua) nao casa o nome
		{FoodID: "6", DataSource: "TACO", OriginalName: "Água, mineral"},
		{FoodID: "7", DataSource: "TACO", OriginalName: "Leite, de vaca, integral", EnergyKcal: 61, ProteinG: 2.9},
	}
	junk, suspects := classify(foods)

	if got := ids(junk); got != "1,2,3" {
		t.Errorf("junk = %s, want 1,2,3", got)
	}
	if got := ids(suspects); got != "4" {
		t.Errorf("suspects = %s, want 4", got)
	}
}

func ids(foods []client.TacoFoodItem) string {
	s := ""
	for i, f := range foods {
		if i > 0 {
			s += ","
		}
		s += f.FoodID
	}
	return s
}

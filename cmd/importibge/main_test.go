package main

import (
	"testing"

	"saas-nutri/internal/client"
	"saas-nutri/internal/ibge"
)

func TestPlan(t *testing.T) {
	table, err := ibge.Load()
	if err != nil {
		t.Fatal(err)
	}
	foods := []client.TacoFoodItem{
		{FoodID: "1", OriginalName: "Queijo, mozarela", DataSource: "TACO"},
		{FoodID: "2", OriginalName: "Arroz, tipo 1, cru", DataSource: "TACO"},
		{FoodID: "3", OriginalName: "Mussarela (Italac)", DataSource: "OFF"},
		{FoodID: "4", OriginalName: "Queijo, prato", DataSource: "TACO"},
		{FoodID: "5", OriginalName: "Queijo, parmesão", DataSource: "TACO"},
	}
	overrides := map[string]override{
		"4": {skip: true},
		"5": {foodCode: 7901801, prepCode: 99}, // forca QUEIJO MUZARELLA
	}
	rows, measures := plan(table, foods, overrides, false)

	want := []string{statusMatched, statusUnmatched, statusSkipped, statusSkipped, statusOverride}
	for i, r := range rows {
		if r.Status != want[i] {
			t.Errorf("%s: status %s, want %s", r.Name, r.Status, want[i])
		}
	}

	byFood := map[string]int{}
	for _, m := range measures {
		byFood[m.FoodID]++
		if m.MeasureSource != ibge.Source || m.MeasureQuantity != "1" {
			t.Errorf("medida sem origem/quantidade: %+v", m)
		}
		if m.FoodID == "1" && m.MeasureName == "fatia" && m.GramEquivalent != 20 {
			t.Errorf("fatia de mucarela = %v g, want 20", m.GramEquivalent)
		}
	}
	if byFood["1"] == 0 || byFood["5"] == 0 || byFood["2"] != 0 || byFood["3"] != 0 || byFood["4"] != 0 {
		t.Errorf("medidas por alimento = %v", byFood)
	}
}

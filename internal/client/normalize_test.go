package client

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestNormalizeTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"vazio", "", nil},
		{"so espacos", "   ", nil},
		{"so pontuacao", " , - ; ", nil},
		{"acento", "requeijão", []string{"requeijao"}},
		{"caixa alta", "REQUEIJÃO", []string{"requeijao"}},
		{"virgulas e hifen", "Queijo, requeijão, cremoso", []string{"queijo", "requeijao", "cremoso"}},
		{"palavra no meio do nome", "Leite de vaca, integral", []string{"leite", "de", "vaca", "integral"}},
		{"digitos preservados", "Refrigerante tipo cola 2L", []string{"refrigerante", "tipo", "cola", "2l"}},
		{"varios acentos", "Pão, francês", []string{"pao", "frances"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeTokens(c.in)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("NormalizeTokens(%q) = %v, quer %v", c.in, got, c.want)
			}
		})
	}
}

func TestSearchTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{",;-", nil},
		{"Requeijão", []string{"requeijao"}},
		{"requeij", []string{"requeij"}},
		{"agua de coco", []string{"agua", "coco"}}, // stopword removida
		{"de", []string{"de"}},                     // so stopword: mantem
	}
	for _, c := range cases {
		got := searchTokens(c.in)
		if len(got) == 0 && len(c.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("searchTokens(%q) = %v, quer %v", c.in, got, c.want)
		}
	}
}

func TestQueryTokenEscolheOMaisLongo(t *testing.T) {
	if got := queryToken([]string{"agua", "coco", "verde"}); got != "verde" {
		t.Errorf("queryToken = %q, quer verde", got)
	}
	if got := queryToken(nil); got != "" {
		t.Errorf("queryToken(nil) = %q, quer vazio", got)
	}
}

func TestFilterRowsByTermsExigeTodasAsPalavras(t *testing.T) {
	rows := []foodTokenItem{
		{FoodID: "tonica", OriginalName: "Água Tônica Hibisco Lata"},
		{FoodID: "coco", OriginalName: "Água De Coco Esterilizada Kero Coco"},
		{FoodID: "doce", OriginalName: "Doce de coco com água"},
	}
	got := dedupeTokenRows(filterRowsByTerms(rows, []string{"agua", "coco"}))
	if len(got) != 2 {
		t.Fatalf("esperava 2 alimentos, veio %d: %+v", len(got), got)
	}
	if got[0].FoodID != "coco" {
		t.Errorf("esperava o nome que comeca com agua primeiro, veio %q", got[0].FoodID)
	}
}

func TestTokenPartition(t *testing.T) {
	cases := map[string]string{
		"requeijao": "r",
		"queijo":    "q",
		"2l":        "_",
		"":          "_",
	}
	for in, want := range cases {
		if got := tokenPartition(in); got != want {
			t.Errorf("tokenPartition(%q) = %q, quer %q", in, got, want)
		}
	}
}

func TestBuildFoodTokenItems(t *testing.T) {
	f := TacoFoodItem{FoodID: "42", OriginalName: "Queijo, requeijão, cremoso", DataSource: "TACO"}
	items := buildFoodTokenItems(f)

	if len(items) != 3 {
		t.Fatalf("esperava 3 tokens, veio %d: %+v", len(items), items)
	}
	want := []struct {
		token     string
		wordIndex int
		partition string
		sort      string
	}{
		{"queijo", 0, "q", "queijo#42"},
		{"requeijao", 1, "r", "requeijao#42"},
		{"cremoso", 2, "c", "cremoso#42"},
	}
	for i, w := range want {
		got := items[i]
		if got.Token != w.token || got.WordIndex != w.wordIndex || got.TokenPartition != w.partition || got.TokenSort != w.sort {
			t.Errorf("item %d = %+v, quer %+v", i, got, w)
		}
	}
}

func TestBuildFoodTokenItemsDedupePalavraRepetida(t *testing.T) {
	f := TacoFoodItem{FoodID: "7", OriginalName: "Feijão, feijão preto"}
	items := buildFoodTokenItems(f)
	if len(items) != 2 {
		t.Fatalf("esperava 2 tokens (feijao, preto), veio %d: %+v", len(items), items)
	}
	if items[0].Token != "feijao" || items[0].WordIndex != 0 {
		t.Errorf("primeiro token = %+v", items[0])
	}
}

func TestDedupeTokenRowsOrdenaPrefixoDoNomePrimeiro(t *testing.T) {
	rows := []foodTokenItem{
		{FoodID: "a", Token: "requeijao", WordIndex: 1, OriginalName: "Queijo, requeijão, cremoso"},
		{FoodID: "b", Token: "requeijao", WordIndex: 0, OriginalName: "Requeijão light"},
		{FoodID: "a", Token: "requeijao", WordIndex: 1, OriginalName: "Queijo, requeijão, cremoso"},
	}
	got := dedupeTokenRows(rows)
	if len(got) != 2 {
		t.Fatalf("esperava 2 alimentos distintos, veio %d", len(got))
	}
	if got[0].FoodID != "b" {
		t.Errorf("esperava o match de inicio de nome (b) primeiro, veio %q", got[0].FoodID)
	}
}

func TestFoodTokenKeys(t *testing.T) {
	keys := FoodTokenKeys(TacoFoodItem{FoodID: "abc", OriginalName: "Descrição dos alimentos dos"})
	want := [][2]string{{"d", "descricao#abc"}, {"d", "dos#abc"}, {"a", "alimentos#abc"}}
	if len(keys) != len(want) {
		t.Fatalf("got %d keys, want %d", len(keys), len(want))
	}
	for i, w := range want {
		pl := keys[i]["token_pl"].(*types.AttributeValueMemberS).Value
		tok := keys[i]["tok"].(*types.AttributeValueMemberS).Value
		if pl != w[0] || tok != w[1] {
			t.Errorf("key %d = (%s, %s), want (%s, %s)", i, pl, tok, w[0], w[1])
		}
	}
}

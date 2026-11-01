// Command importibge carrega na HouseholdMeasures as medidas caseiras da
// Tabela de Medidas Referidas para os Alimentos Consumidos no Brasil (IBGE,
// POF 2008-2009), embutida em internal/ibge.
//
// Percorre a TacoFoods, casa o nome de cada alimento TACO com a tabela do IBGE
// (ver ibge.Table.Match) e grava as medidas do alimento casado com
// measure_source = "IBGE POF 2008-2009". Sempre escreve um relatorio CSV com o
// casamento de cada alimento para revisao; com -dry-run nao grava nada.
//
// E idempotente e nao apaga nada: cada medida sobrescreve so a propria chave
// (food_id, measure_name). Na API, um alimento com medidas do IBGE passa a
// exibir apenas a medida base e as do IBGE (o trio generico antigo e as
// estimativas por tipo de alimento deixam de aparecer para ele).
//
// Casamentos errados se corrigem com -overrides, um CSV
// "food_id,ibge_food_code,ibge_prep_code" (codigos do IBGE do relatorio ou
// de internal/ibge/medidas_pof2008.csv; codigos vazios = nao importar).
//
// Uso:
//
//	go run ./cmd/importibge -dry-run -report ibge_report.csv
//	go run ./cmd/importibge -overrides ibge_overrides.csv
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"saas-nutri/internal/client"
	"saas-nutri/internal/ibge"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/joho/godotenv"
)

const (
	tacoTable = "TacoFoods"
	awsRegion = "sa-east-1"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "casa e escreve o relatorio sem gravar no DynamoDB")
	reportPath := flag.String("report", "ibge_report.csv", "caminho do relatorio CSV de casamentos")
	overridesPath := flag.String("overrides", "", "CSV food_id,ibge_food_code,ibge_prep_code para corrigir casamentos")
	includeOFF := flag.Bool("include-off", false, "tambem casa produtos do Open Food Facts (padrao: so TACO)")
	flag.Parse()

	table, err := ibge.Load()
	if err != nil {
		log.Fatalf("erro ao carregar a tabela do IBGE: %v", err)
	}
	overrides := map[string]override{}
	if *overridesPath != "" {
		if overrides, err = loadOverrides(*overridesPath); err != nil {
			log.Fatal(err)
		}
	}

	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: .env nao encontrado; usando credenciais do ambiente.")
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("erro ao carregar configuracao AWS: %v", err)
	}
	db := dynamodb.NewFromConfig(cfg)
	repo := client.NewTacoRepository(db, tacoTable, "FoodSearchTokens")

	foods, err := scanFoods(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	rows, measures := plan(table, foods, overrides, *includeOFF)

	if err := writeReport(*reportPath, rows); err != nil {
		log.Fatal(err)
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Status]++
	}
	log.Printf("alimentos: %d | casados: %d | override: %d | sem casamento: %d | ignorados: %d | medidas: %d",
		len(rows), counts[statusMatched], counts[statusOverride], counts[statusUnmatched], counts[statusSkipped], len(measures))
	log.Printf("relatorio: %s", *reportPath)

	if *dryRun {
		log.Println("dry-run: nada gravado na HouseholdMeasures")
		return
	}
	// So a gravacao depende da chave da tabela; requer dynamodb:DescribeTable.
	if err := repo.CheckMeasuresKeySchema(ctx); err != nil {
		log.Fatal(err)
	}
	if err := repo.PutMeasuresBatch(ctx, measures); err != nil {
		log.Fatalf("erro ao gravar medidas: %v", err)
	}
	log.Printf("%d medidas gravadas na %s", len(measures), client.HouseholdMeasuresTable)
}

const (
	statusMatched   = "casado"
	statusOverride  = "override"
	statusUnmatched = "sem_casamento"
	statusSkipped   = "ignorado"
)

type override struct {
	foodCode, prepCode int
	skip               bool
}

type reportRow struct {
	FoodID, Name, Source, Status string
	Entry                        *ibge.Entry
	Score                        float64
}

// plan casa cada alimento com a tabela do IBGE e monta as medidas a gravar.
func plan(table *ibge.Table, foods []client.TacoFoodItem, overrides map[string]override, includeOFF bool) ([]reportRow, []client.MeasureItem) {
	var rows []reportRow
	var measures []client.MeasureItem
	for _, f := range foods {
		row := reportRow{FoodID: f.FoodID, Name: f.OriginalName, Source: f.DataSource}
		if o, ok := overrides[f.FoodID]; ok {
			row.Status = statusSkipped
			if !o.skip {
				row.Entry = table.Entry(o.foodCode, o.prepCode)
				row.Status = statusOverride
				if row.Entry == nil {
					log.Printf("aviso: override de %s aponta para codigo IBGE inexistente %d/%d", f.FoodID, o.foodCode, o.prepCode)
					row.Status = statusUnmatched
				}
			}
		} else if f.DataSource == "OFF" && !includeOFF {
			row.Status = statusSkipped
		} else {
			row.Entry, row.Score = table.Match(f.OriginalName)
			row.Status = statusUnmatched
			if row.Entry != nil {
				row.Status = statusMatched
			}
		}
		if row.Entry != nil {
			for _, m := range row.Entry.Measures {
				measures = append(measures, client.MeasureItem{
					FoodID:          f.FoodID,
					MeasureName:     m.Name,
					MeasureQuantity: "1",
					GramEquivalent:  m.Grams,
					MeasureSource:   ibge.Source,
				})
			}
		}
		rows = append(rows, row)
	}
	return rows, measures
}

func scanFoods(ctx context.Context, db *dynamodb.Client) ([]client.TacoFoodItem, error) {
	var (
		foods    []client.TacoFoodItem
		startKey map[string]types.AttributeValue
	)
	for {
		out, err := db.Scan(ctx, &dynamodb.ScanInput{
			TableName:            aws.String(tacoTable),
			ProjectionExpression: aws.String("food_id, original_name, data_source"),
			ExclusiveStartKey:    startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("erro ao percorrer %s: %w", tacoTable, err)
		}
		var page []client.TacoFoodItem
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, fmt.Errorf("erro ao ler alimentos: %w", err)
		}
		foods = append(foods, page...)
		if len(out.LastEvaluatedKey) == 0 {
			return foods, nil
		}
		startKey = out.LastEvaluatedKey
	}
}

func loadOverrides(path string) (map[string]override, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir overrides %s: %w", path, err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler overrides %s: %w", path, err)
	}
	out := map[string]override{}
	for i, rec := range recs {
		if len(rec) != 3 {
			return nil, fmt.Errorf("overrides linha %d: esperado food_id,ibge_food_code,ibge_prep_code", i+1)
		}
		if i == 0 && rec[0] == "food_id" {
			continue
		}
		id := strings.TrimSpace(rec[0])
		if strings.TrimSpace(rec[1]) == "" {
			out[id] = override{skip: true}
			continue
		}
		food, err1 := strconv.Atoi(strings.TrimSpace(rec[1]))
		prep, err2 := strconv.Atoi(strings.TrimSpace(rec[2]))
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("overrides linha %d: codigos do IBGE invalidos %v", i+1, rec)
		}
		out[id] = override{foodCode: food, prepCode: prep}
	}
	return out, nil
}

func writeReport(path string, rows []reportRow) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("erro ao criar relatorio %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	w.Write([]string{"food_id", "nome", "origem", "status", "ibge_food_code", "ibge_alimento", "ibge_prep_code", "ibge_preparo", "placar", "medidas"})
	for _, r := range rows {
		rec := []string{r.FoodID, r.Name, r.Source, r.Status, "", "", "", "", "", ""}
		if e := r.Entry; e != nil {
			var ms []string
			for _, m := range e.Measures {
				ms = append(ms, fmt.Sprintf("%s=%sg", m.Name, strconv.FormatFloat(m.Grams, 'f', -1, 64)))
			}
			rec[4], rec[5] = strconv.Itoa(e.FoodCode), e.Food
			rec[6], rec[7] = strconv.Itoa(e.PrepCode), e.Prep
			rec[8] = strconv.FormatFloat(r.Score, 'f', 2, 64)
			rec[9] = strings.Join(ms, "; ")
		}
		w.Write(rec)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return fmt.Errorf("erro ao escrever relatorio %s: %w", path, err)
	}
	return f.Close()
}

package client

import (
	"context"
	"fmt"
	"log"
	"saas-nutri/internal/model"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type TacoFoodItem struct {
	FoodID          string  `dynamodbav:"food_id"`
	DataSource      string  `dynamodbav:"data_source"`
	NormalizedName  string  `dynamodbav:"normalized_name"`
	OriginalName    string  `dynamodbav:"original_name"`
	EnergyKcal      float64 `dynamodbav:"energy_kcal,omitempty"`
	ProteinG        float64 `dynamodbav:"protein_g,omitempty"`
	CarbohydrateG   float64 `dynamodbav:"carbohydrate_g,omitempty"`
	FatG            float64 `dynamodbav:"fat_g,omitempty"`
	FiberG          float64 `dynamodbav:"fiber_g,omitempty"`
}

type TacoRepository struct {
	DB        *dynamodb.Client
	TableName string
	// SearchTokensTableName e a tabela do indice de palavras (uma linha por
	// token de cada alimento) consultada pela busca por prefixo.
	SearchTokensTableName string
}

// foodTokenItem e uma linha da tabela de tokens de busca. Os campos do alimento
// sao desnormalizados aqui para a busca resolver tudo numa unica Query, sem
// segundo round-trip na TacoFoods.
type foodTokenItem struct {
	TokenPartition string  `dynamodbav:"token_pl"`
	TokenSort      string  `dynamodbav:"tok"`
	Token          string  `dynamodbav:"token"`
	WordIndex      int     `dynamodbav:"word_index"`
	FoodID         string  `dynamodbav:"food_id"`
	DataSource     string  `dynamodbav:"data_source"`
	OriginalName   string  `dynamodbav:"original_name"`
	NormalizedName string  `dynamodbav:"normalized_name"`
	EnergyKcal     float64 `dynamodbav:"energy_kcal"`
	ProteinG       float64 `dynamodbav:"protein_g"`
	CarbohydrateG  float64 `dynamodbav:"carbohydrate_g"`
	FatG           float64 `dynamodbav:"fat_g"`
	FiberG         float64 `dynamodbav:"fiber_g"`
}

// buildFoodTokenItems gera as linhas de token para um alimento. Usa a mesma
// NormalizeTokens da busca; palavras repetidas no nome viram uma linha so.
func buildFoodTokenItems(f TacoFoodItem) []foodTokenItem {
	tokens := NormalizeTokens(f.OriginalName)
	seen := make(map[string]bool, len(tokens))
	items := make([]foodTokenItem, 0, len(tokens))
	for i, tk := range tokens {
		if seen[tk] {
			continue
		}
		seen[tk] = true
		items = append(items, foodTokenItem{
			TokenPartition: tokenPartition(tk),
			TokenSort:      tk + "#" + f.FoodID,
			Token:          tk,
			WordIndex:      i,
			FoodID:         f.FoodID,
			DataSource:     f.DataSource,
			OriginalName:   f.OriginalName,
			NormalizedName: f.NormalizedName,
			EnergyKcal:     f.EnergyKcal,
			ProteinG:       f.ProteinG,
			CarbohydrateG:  f.CarbohydrateG,
			FatG:           f.FatG,
			FiberG:         f.FiberG,
		})
	}
	return items
}

type MeasureItem struct {
    FoodID          string  `json:"-" dynamodbav:"food_id"`
    MeasureName     string  `json:"measure_name" dynamodbav:"measure_name"`
    MeasureQuantity string  `json:"measure_quantity" dynamodbav:"measure_quantity"`
    DisplayName     string  `json:"display_name"`
    GramEquivalent  float64 `json:"gram_equivalent" dynamodbav:"measure_weight_g"`
}

func NewTacoRepository(db *dynamodb.Client, tableName, searchTokensTableName string) *TacoRepository {
	return &TacoRepository{
		DB:                    db,
		TableName:             tableName,
		SearchTokensTableName: searchTokensTableName,
	}
}

// searchResultLimit e o teto de alimentos distintos devolvidos pela busca.
const searchResultLimit = 25

// SearchFoodsByNamePrefix busca alimentos cujo termo casa o prefixo de
// qualquer palavra do nome (ex.: "requeij" acha "Queijo, requeijão, cremoso").
//
// A consulta e uma Query por prefixo na tabela de tokens (nunca Scan na
// TacoFoods): particao = primeira letra do token, sort key = "<token>#<food_id>".
// Resultados sao deduplicados por alimento e os que casam o inicio do nome
// (primeira palavra) vem primeiro. Termo vazio devolve lista vazia.
func (r *TacoRepository) SearchFoodsByNamePrefix(ctx context.Context, term string) ([]TacoFoodItem, error) {
	token := searchToken(term)
	if token == "" {
		return []TacoFoodItem{}, nil
	}

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(r.SearchTokensTableName),
		KeyConditionExpression: aws.String("token_pl = :p AND begins_with(tok, :t)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":p": &types.AttributeValueMemberS{Value: tokenPartition(token)},
			":t": &types.AttributeValueMemberS{Value: token},
		},
		Limit: aws.Int32(200),
	}

	log.Printf("Executando Query na tabela de tokens '%s' com prefixo: '%s'", r.SearchTokensTableName, token)

	result, err := r.DB.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar query no DynamoDB: %w", err)
	}

	var rows []foodTokenItem
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &rows); err != nil {
		return nil, fmt.Errorf("erro ao fazer unmarshal dos resultados do DynamoDB: %w", err)
	}

	items := dedupeTokenRows(rows)
	log.Printf("Busca por token '%s' retornou %d alimentos distintos", token, len(items))
	return items, nil
}

// dedupeTokenRows colapsa varias linhas de token do mesmo alimento em um item,
// mantendo o menor word_index que casou, e ordena os que casam a primeira
// palavra do nome antes dos demais (ordem estavel dentro de cada grupo).
func dedupeTokenRows(rows []foodTokenItem) []TacoFoodItem {
	order := make([]string, 0, len(rows))
	best := make(map[string]foodTokenItem, len(rows))
	for _, row := range rows {
		cur, ok := best[row.FoodID]
		if !ok {
			best[row.FoodID] = row
			order = append(order, row.FoodID)
			continue
		}
		if row.WordIndex < cur.WordIndex {
			best[row.FoodID] = row
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		return matchRank(best[order[i]]) < matchRank(best[order[j]])
	})

	items := make([]TacoFoodItem, 0, len(order))
	for _, id := range order {
		if len(items) >= searchResultLimit {
			break
		}
		row := best[id]
		items = append(items, TacoFoodItem{
			FoodID:         row.FoodID,
			DataSource:     row.DataSource,
			NormalizedName: row.NormalizedName,
			OriginalName:   row.OriginalName,
			EnergyKcal:     row.EnergyKcal,
			ProteinG:       row.ProteinG,
			CarbohydrateG:  row.CarbohydrateG,
			FatG:           row.FatG,
			FiberG:         row.FiberG,
		})
	}
	return items
}

// matchRank: 0 quando o termo casa a primeira palavra do nome, 1 caso contrario.
func matchRank(row foodTokenItem) int {
	if row.WordIndex == 0 {
		return 0
	}
	return 1
}

// PutFood grava um alimento na TacoFoods e (re)grava suas linhas de token no
// indice de busca, para o alimento ja ser encontrado pelas palavras do nome
// sem depender do backfill.
func (r *TacoRepository) PutFood(ctx context.Context, f TacoFoodItem) error {
	av, err := attributevalue.MarshalMap(f)
	if err != nil {
		return fmt.Errorf("erro ao serializar alimento %s: %w", f.FoodID, err)
	}
	if _, err := r.DB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.TableName),
		Item:      av,
	}); err != nil {
		return fmt.Errorf("erro ao gravar alimento %s: %w", f.FoodID, err)
	}
	return r.PutFoodTokens(ctx, f)
}

// PutFoodTokens grava apenas as linhas de token de um alimento. E idempotente:
// as chaves sao deterministicas, entao reexecutar sobrescreve linhas iguais.
// Usado pelo PutFood e pela rotina de backfill.
func (r *TacoRepository) PutFoodTokens(ctx context.Context, f TacoFoodItem) error {
	tokenItems := buildFoodTokenItems(f)
	if len(tokenItems) == 0 {
		return nil
	}

	requests := make([]types.WriteRequest, 0, len(tokenItems))
	for _, ti := range tokenItems {
		av, err := attributevalue.MarshalMap(ti)
		if err != nil {
			return fmt.Errorf("erro ao serializar token do alimento %s: %w", f.FoodID, err)
		}
		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: av},
		})
	}
	return r.batchWrite(ctx, r.SearchTokensTableName, requests)
}

// PutFoodsBatch grava um conjunto de alimentos na TacoFoods e as linhas de
// token de busca de cada um, em lote, com o mesmo tratamento de throttling do
// batchWrite. Cada alimento sobrescreve apenas a propria chave food_id: itens
// de outra origem (ex.: TACO) nao sao lidos nem alterados. Usado pela carga do
// Open Food Facts (cmd/importoff).
func (r *TacoRepository) PutFoodsBatch(ctx context.Context, foods []TacoFoodItem) error {
	if len(foods) == 0 {
		return nil
	}

	foodRequests := make([]types.WriteRequest, 0, len(foods))
	tokenRequests := make([]types.WriteRequest, 0, len(foods)*6)
	for _, f := range foods {
		av, err := attributevalue.MarshalMap(f)
		if err != nil {
			return fmt.Errorf("erro ao serializar alimento %s: %w", f.FoodID, err)
		}
		foodRequests = append(foodRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: av},
		})

		for _, ti := range buildFoodTokenItems(f) {
			tav, err := attributevalue.MarshalMap(ti)
			if err != nil {
				return fmt.Errorf("erro ao serializar token do alimento %s: %w", f.FoodID, err)
			}
			tokenRequests = append(tokenRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: tav},
			})
		}
	}

	if err := r.batchWrite(ctx, r.TableName, foodRequests); err != nil {
		return err
	}
	return r.batchWrite(ctx, r.SearchTokensTableName, tokenRequests)
}

// batchWrite envia WriteRequests em lotes de 25, reprocessando os itens que o
// DynamoDB devolver como UnprocessedItems com backoff exponencial (tabela
// on-demand recem-criada costuma throttlar rajadas ate escalar a capacidade).
func (r *TacoRepository) batchWrite(ctx context.Context, table string, requests []types.WriteRequest) error {
	const (
		maxBatch    = 25
		maxAttempts = 8
	)
	for start := 0; start < len(requests); start += maxBatch {
		end := start + maxBatch
		if end > len(requests) {
			end = len(requests)
		}
		pending := requests[start:end]
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == maxAttempts {
				return fmt.Errorf("batch write em %s deixou %d itens nao processados apos %d tentativas", table, len(pending), maxAttempts)
			}
			if attempt > 0 {
				wait := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			out, err := r.DB.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{table: pending},
			})
			if err != nil {
				return fmt.Errorf("erro no batch write em %s: %w", table, err)
			}
			pending = out.UnprocessedItems[table]
		}
	}
	return nil
}

func (r *TacoRepository) GetMeasuresForFood(ctx context.Context, foodID string) ([]MeasureItem, error) {
    var items []MeasureItem
    defaultMeasure := MeasureItem{
        MeasureName:    BaseMeasureGram,
        DisplayName:    "Grama (1g)",
        GramEquivalent: 1.0,
    }
    liquid := false

    // Get food name to determine category and add appropriate measures
    foodKey := map[string]types.AttributeValue{
        "food_id": &types.AttributeValueMemberS{Value: foodID},
    }
    foodResult, err := r.DB.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(r.TableName),
        Key:       foodKey,
        ProjectionExpression: aws.String("original_name"),
    })
    if err != nil {
        log.Printf("Erro ao buscar nome do alimento: %v", err)
    } else if foodResult.Item != nil {
        var food TacoFoodItem
        err = attributevalue.UnmarshalMap(foodResult.Item, &food)
        if err == nil {
            foodName := strings.ToLower(food.OriginalName)
            liquid = IsLiquid(food.OriginalName)
            if liquid {
                defaultMeasure = MeasureItem{
                    MeasureName:    BaseMeasureMl,
                    DisplayName:    "Mililitro (1mL)",
                    GramEquivalent: 1.0,
                }
            }
            // Add measures based on food name keywords for categories
            if liquid {
                // liquidos usam apenas mL e as medidas caseiras do banco
            } else if isFruit(foodName) {
                fruitMeasures := []MeasureItem{
                    {FoodID: foodID, MeasureName: "fatia pequena (30g)", MeasureQuantity: "", DisplayName: "fatia pequena (30g)", GramEquivalent: 30.0},
                    {FoodID: foodID, MeasureName: "fatia média (50g)", MeasureQuantity: "", DisplayName: "fatia média (50g)", GramEquivalent: 50.0},
                    {FoodID: foodID, MeasureName: "fatia grande (70g)", MeasureQuantity: "", DisplayName: "fatia grande (70g)", GramEquivalent: 70.0},
                    {FoodID: foodID, MeasureName: "unidade média (100g)", MeasureQuantity: "", DisplayName: "unidade média (100g)", GramEquivalent: 100.0},
                }
                items = append(items, fruitMeasures...)
            } else if isMeat(foodName) {
                meatMeasures := []MeasureItem{
                    {FoodID: foodID, MeasureName: "fatia pequena (50g)", MeasureQuantity: "", DisplayName: "fatia pequena (50g)", GramEquivalent: 50.0},
                    {FoodID: foodID, MeasureName: "fatia média (100g)", MeasureQuantity: "", DisplayName: "fatia média (100g)", GramEquivalent: 100.0},
                    {FoodID: foodID, MeasureName: "fatia grande (150g)", MeasureQuantity: "", DisplayName: "fatia grande (150g)", GramEquivalent: 150.0},
                    {FoodID: foodID, MeasureName: "porção pequena (100g)", MeasureQuantity: "", DisplayName: "porção pequena (100g)", GramEquivalent: 100.0},
                    {FoodID: foodID, MeasureName: "porção média (150g)", MeasureQuantity: "", DisplayName: "porção média (150g)", GramEquivalent: 150.0},
                    {FoodID: foodID, MeasureName: "porção grande (200g)", MeasureQuantity: "", DisplayName: "porção grande (200g)", GramEquivalent: 200.0},
                }
                items = append(items, meatMeasures...)
            }
        }
    }

    items = append([]MeasureItem{defaultMeasure}, items...)

    keyConditionExpression := "food_id = :fid"
    expressionAttributeValues := map[string]types.AttributeValue{
        ":fid": &types.AttributeValueMemberS{Value: foodID},
    }
    
    projectionExpression := "food_id, measure_name, measure_quantity, measure_weight_g"

    log.Printf("Buscando medidas para food_id: %s", foodID)

    queryInput := &dynamodb.QueryInput{
        TableName:                 aws.String("HouseholdMeasures"),
        KeyConditionExpression:    aws.String(keyConditionExpression),
        ExpressionAttributeValues: expressionAttributeValues,
        ProjectionExpression:      aws.String(projectionExpression),
    }

    result, err := r.DB.Query(ctx, queryInput)
    if err != nil {
        log.Printf("ERRO ao buscar medidas: %v", err)
        return nil, fmt.Errorf("erro ao buscar medidas no DB para food_id %s: %w", foodID, err)
    }

    log.Printf("DynamoDB retornou %d itens", len(result.Items))
    for i, item := range result.Items {
        log.Printf("Item %d: %+v", i, item)
    }

    var dbMeasures []MeasureItem
    err = attributevalue.UnmarshalListOfMaps(result.Items, &dbMeasures)
    if err != nil {
        log.Printf("ERRO no unmarshal: %v", err)
        return nil, fmt.Errorf("erro ao processar medidas do DB para food_id %s: %w", foodID, err)
    }

    log.Printf("Após unmarshal: %+v", dbMeasures)

    for i := range dbMeasures {
        dbMeasures[i].DisplayName = dbMeasures[i].MeasureName
        log.Printf("Medida %d: name=%s, quantity=%s, weight=%f, display=%s", 
            i, dbMeasures[i].MeasureName, dbMeasures[i].MeasureQuantity, 
            dbMeasures[i].GramEquivalent, dbMeasures[i].DisplayName)
    }

    // Create a map of existing measure_names to avoid duplicates
    existing := make(map[string]bool)
    for _, m := range items {
        existing[m.MeasureName] = true
    }

    // Filter dbMeasures to avoid duplicates
    var uniqueDbMeasures []MeasureItem
    for _, m := range dbMeasures {
        if liquid && m.MeasureName == BaseMeasureGram {
            continue
        }
        if !existing[m.MeasureName] {
            uniqueDbMeasures = append(uniqueDbMeasures, m)
            existing[m.MeasureName] = true
        }
    }

    items = append(items, uniqueDbMeasures...)
    return items, nil
}

func isFruit(name string) bool {
    fruits := []string{"banana", "maçã", "laranja", "abacaxi", "uva", "pera", "manga", "melancia", "morango", "fruta"}
    for _, fruit := range fruits {
        if strings.Contains(name, fruit) {
            return true
        }
    }
    return false
}

func isMeat(name string) bool {
    meats := []string{"bife", "carne", "frango", "peixe", "bovina", "suína", "aves", "cordeiro"}
    for _, meat := range meats {
        if strings.Contains(name, meat) {
            return true
        }
    }
    return false
}

func (r *TacoRepository) GetFoodWithMeasures(ctx context.Context, foodID string) (*model.Food, error) {
	key := map[string]types.AttributeValue{
		"food_id": &types.AttributeValueMemberS{Value: foodID},
	}

	getItemInput := &dynamodb.GetItemInput{
		TableName: aws.String(r.TableName),
		Key:       key,
	}

	result, err := r.DB.GetItem(ctx, getItemInput)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar alimento no DynamoDB: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("alimento não encontrado: %s", foodID)
	}

	var foodItem TacoFoodItem
	err = attributevalue.UnmarshalMap(result.Item, &foodItem)
	if err != nil {
		return nil, fmt.Errorf("erro ao deserializar alimento: %w", err)
	}

	food := &model.Food{
		Id:            foodItem.FoodID,
		Name:          foodItem.NormalizedName, 
		Source:        foodItem.DataSource,
		Unit:          UnitForFood(foodItem.OriginalName),
		EnergyKcal:    foodItem.EnergyKcal,
		ProteinG:      foodItem.ProteinG,
		CarbohydrateG: foodItem.CarbohydrateG,
		FatG:          foodItem.FatG,
		FiberG:        foodItem.FiberG,
	}

	measures, err := r.GetMeasuresForFood(ctx, foodID)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar medidas caseiras: %w", err)
	}

	var householdMeasures []model.HouseholdMeasure
	for _, m := range measures {
		householdMeasures = append(householdMeasures, model.HouseholdMeasure{
			Name:  m.MeasureName,
			Grams: m.GramEquivalent,
		})
	}

	food.HouseholdMeasures = householdMeasures

	return food, nil
}
// Command cleanupjunk remove da TacoFoods (e da FoodSearchTokens) as linhas com
// data_source "TACO" que nao sao alimentos: cabecalhos e notas de rodape que o
// parser do PDF da TACO gravou como se fossem itens ("Descricao dos alimentos",
// "Limites de Quantificacao: ...", "L", etc.).
//
// A regra e explicita (ver isJunk): o nome precisa casar uma lista conhecida E
// todos os nutrientes precisam estar zerados. Linhas com nome suspeito mas com
// nutrientes sao apenas reportadas, nunca apagadas.
//
// E idempotente: rodar de novo depois da limpeza nao encontra nada.
//
//	go run ./cmd/cleanupjunk -dry-run   # so lista food_id e nome
//	go run ./cmd/cleanupjunk            # apaga
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"time"

	"saas-nutri/internal/client"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/joho/godotenv"
)

const (
	tacoTable   = "TacoFoods"
	tokensTable = "FoodSearchTokens"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "lista as linhas-lixo sem apagar nada")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: .env nao encontrado; usando credenciais do ambiente.")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("sa-east-1"))
	if err != nil {
		log.Fatalf("erro ao carregar configuracao AWS: %v", err)
	}
	db := dynamodb.NewFromConfig(cfg)

	var foods []client.TacoFoodItem
	err = scanAll(ctx, db, tacoTable, func(items []map[string]types.AttributeValue) error {
		var page []client.TacoFoodItem
		if err := attributevalue.UnmarshalListOfMaps(items, &page); err != nil {
			return err
		}
		foods = append(foods, page...)
		return nil
	})
	if err != nil {
		log.Fatalf("erro ao percorrer %s: %v", tacoTable, err)
	}

	junk, suspects := classify(foods)
	sort.Slice(junk, func(i, j int) bool { return junk[i].OriginalName < junk[j].OriginalName })

	fmt.Printf("%d alimentos lidos; %d linhas-lixo:\n", len(foods), len(junk))
	for _, f := range junk {
		fmt.Printf("  %s  %q\n", f.FoodID, f.OriginalName)
	}
	if len(suspects) > 0 {
		fmt.Printf("%d linhas com nome de lixo mas nutrientes preenchidos (NAO serao apagadas):\n", len(suspects))
		for _, f := range suspects {
			fmt.Printf("  %+v\n", f)
		}
	}

	// Apenas informativo: ajuda a ver se algum cabecalho novo escapou da lista.
	for _, f := range foods {
		if f.DataSource == "TACO" && zeroNutrients(f) && !isJunk(f) {
			fmt.Printf("  (fora da regra, nutrientes zerados) %s  %q\n", f.FoodID, f.OriginalName)
		}
	}

	// As chaves de token sao deterministicas (token#food_id), entao basta
	// recalcula-las a partir do nome; a Query so confirma quais existem e evita
	// precisar de Scan na FoodSearchTokens.
	var tokenKeys []map[string]types.AttributeValue
	for _, f := range junk {
		for _, k := range client.FoodTokenKeys(f) {
			out, err := db.Query(ctx, &dynamodb.QueryInput{
				TableName:              aws.String(tokensTable),
				KeyConditionExpression: aws.String("token_pl = :p AND tok = :t"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":p": k["token_pl"],
					":t": k["tok"],
				},
				Select: types.SelectCount,
			})
			if err != nil {
				log.Fatalf("erro ao consultar %s: %v", tokensTable, err)
			}
			if out.Count > 0 {
				tokenKeys = append(tokenKeys, k)
			}
		}
	}
	fmt.Printf("%d linhas correspondentes na %s\n", len(tokenKeys), tokensTable)

	if *dryRun {
		log.Println("dry-run: nada foi apagado.")
		return
	}
	if len(junk) == 0 && len(tokenKeys) == 0 {
		log.Println("nada a apagar.")
		return
	}

	// Tokens primeiro: se o processo cair no meio, o alimento ainda existe e
	// uma nova execucao encontra e termina o trabalho.
	if err := batchDelete(ctx, db, tokensTable, tokenKeys); err != nil {
		log.Fatal(err)
	}
	foodKeys := make([]map[string]types.AttributeValue, 0, len(junk))
	for _, f := range junk {
		foodKeys = append(foodKeys, map[string]types.AttributeValue{
			"food_id": &types.AttributeValueMemberS{Value: f.FoodID},
		})
	}
	if err := batchDelete(ctx, db, tacoTable, foodKeys); err != nil {
		log.Fatal(err)
	}
	log.Printf("limpeza concluida: %d alimentos e %d tokens apagados", len(foodKeys), len(tokenKeys))
}

func scanAll(ctx context.Context, db *dynamodb.Client, table string, fn func([]map[string]types.AttributeValue) error) error {
	var startKey map[string]types.AttributeValue
	for {
		out, err := db.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(table),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return err
		}
		if err := fn(out.Items); err != nil {
			return err
		}
		if len(out.LastEvaluatedKey) == 0 {
			return nil
		}
		startKey = out.LastEvaluatedKey
	}
}

// batchDelete apaga as chaves em lotes de 25, reenviando UnprocessedItems.
func batchDelete(ctx context.Context, db *dynamodb.Client, table string, keys []map[string]types.AttributeValue) error {
	const maxBatch = 25
	for start := 0; start < len(keys); start += maxBatch {
		end := min(start+maxBatch, len(keys))
		pending := make([]types.WriteRequest, 0, end-start)
		for _, k := range keys[start:end] {
			pending = append(pending, types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: k}})
		}
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt == 8 {
				return fmt.Errorf("delete em %s deixou %d itens nao processados", table, len(pending))
			}
			if attempt > 0 {
				time.Sleep(time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond)
			}
			out, err := db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{table: pending},
			})
			if err != nil {
				return fmt.Errorf("erro ao apagar em %s: %w", table, err)
			}
			pending = out.UnprocessedItems[table]
		}
	}
	return nil
}

package handler

import (
	"log"
	"net/http"
	"strings"

	"saas-nutri/internal/client"
	"saas-nutri/internal/model"

	"github.com/go-chi/chi/v5"
)

type FoodHandler struct {
	tacoRepo *client.TacoRepository
}

func NewFoodHandler(taco *client.TacoRepository) *FoodHandler {
	return &FoodHandler{
		tacoRepo: taco,
	}
}

func mapTacoToFood(tacoItem client.TacoFoodItem) model.Food {
	source := tacoItem.DataSource
	if source == "" {
		source = "TACO"
	}
	return model.Food{
		Id:            tacoItem.FoodID,
		Name:          tacoItem.OriginalName,
		Source:        source,
		Unit:          client.UnitForFood(tacoItem.OriginalName),
		EnergyKcal:    tacoItem.EnergyKcal,
		ProteinG:      tacoItem.ProteinG,
		CarbohydrateG: tacoItem.CarbohydrateG,
		FatG:          tacoItem.FatG,
		FiberG:        tacoItem.FiberG,
		PortionSizes:  getPortionSizes(tacoItem.OriginalName),
	}
}

func getPortionSizes(name string) []model.PortionSize {
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "fruta") || strings.Contains(nameLower, "fruit") || strings.Contains(nameLower, "maçã") || strings.Contains(nameLower, "banana") {
		return []model.PortionSize{
			{Size: "small", Grams: 50},
			{Size: "medium", Grams: 100},
			{Size: "large", Grams: 150},
		}
	} else if strings.Contains(nameLower, "bife") || strings.Contains(nameLower, "steak") || strings.Contains(nameLower, "carne") {
		return []model.PortionSize{
			{Size: "small", Grams: 100},
			{Size: "medium", Grams: 200},
			{Size: "large", Grams: 300},
		}
	}
	// Default: empty or generic
	return []model.PortionSize{}
}

// SearchFoods godoc
// @Summary      Busca alimentos
// @Description  Busca alimentos na base TACO
// @Tags         alimentos
// @Accept       json
// @Produce      json
// @Param        search query string false "Termo para buscar o alimento" example(arroz)
// @Success      200 {array} model.Food "Lista de alimentos encontrados da TACO (vazia se o termo for vazio ou sem resultados)"
// @Failure      500 {object} string "Erro interno ao buscar dados dos alimentos"
// @Router       /foods [get]

func (h *FoodHandler) SearchFoods(w http.ResponseWriter, r *http.Request) {
	searchTerm := r.URL.Query().Get("search")

	ctx := r.Context()
	mappedResults := make([]model.Food, 0)

	tacoResults, errTaco := h.tacoRepo.SearchFoodsByNamePrefix(ctx, searchTerm)
	if errTaco != nil {
		log.Printf("Erro ao buscar alimentos (search=%q): %v", searchTerm, errTaco)
		RespondWithError(w, http.StatusInternalServerError, "Erro interno ao buscar dados dos alimentos")
		return
	}

	for _, tacoItem := range tacoResults {
		mappedTacoItem := mapTacoToFood(tacoItem)
		mappedResults = append(mappedResults, mappedTacoItem)
	}

	RespondWithJSON(w, http.StatusOK, mappedResults)
}

// GetFoodMeasures godoc
// @Summary      Busca medidas caseiras de um alimento
// @Description  Retorna uma lista de medidas caseiras e seus equivalentes em gramas para um ID de alimento específico.
// @Tags         alimentos
// @Accept       json
// @Produce      json
// @Param        foodId path string true "ID do Alimento (ex: UUID ou código TACO)"
// @Success      200 {array} client.MeasureItem "Lista de medidas caseiras"
// @Failure      400 {object} string "Erro: ID do alimento é obrigatório"
// @Failure      500 {object} string "Erro interno ao buscar medidas"
// @Router       /foods/{foodId}/measures [get]

func (h *FoodHandler) GetFoodMeasures(w http.ResponseWriter, r *http.Request) {
	foodId := chi.URLParam(r, "foodId")
	if foodId == "" {
		RespondWithError(w, http.StatusBadRequest, "ID do alimento é obrigatório na URL")
		return
	}

	ctx := r.Context()
	measures, err := h.tacoRepo.GetMeasuresForFood(ctx, foodId)
	if err != nil {
		RespondWithError(w, http.StatusInternalServerError, "Erro ao buscar medidas caseiras")
		return
	}

	RespondWithJSON(w, http.StatusOK, measures)
}

func (h *FoodHandler) GetFoodWithMeasures(w http.ResponseWriter, r *http.Request) {
	foodId := chi.URLParam(r, "foodId")
	if foodId == "" {
		RespondWithError(w, http.StatusBadRequest, "ID do alimento é obrigatório na URL")
		return
	}

	ctx := r.Context()
	food, err := h.tacoRepo.GetFoodWithMeasures(ctx, foodId)
	if err != nil {
		RespondWithError(w, http.StatusNotFound, "Alimento não encontrado")
		return
	}

	// Adicionar PortionSizes baseado no nome
	food.PortionSizes = getPortionSizes(food.Name)

	RespondWithJSON(w, http.StatusOK, food)
}
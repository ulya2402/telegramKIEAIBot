package core

import (
	"encoding/json"
	"fmt"
	"os"
)

type AIModel struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	APIModelID   string   `json:"api_model_id"`
	Description  string   `json:"description"`
	SupportedOps []string `json:"supported_ops"`
	Ratios       []string `json:"ratios"`
	Resolutions  []string `json:"resolutions"`
	Formats      []string `json:"formats"`
}

type Provider struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Models []AIModel `json:"models"`
}

// Global variable untuk menyimpan data yang dimuat
var AI_REGISTRY []Provider

// LoadRegistry membaca file JSON dan mengisi variabel AI_REGISTRY
func LoadRegistry(filePath string) error {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read models file: %v", err)
	}

	var providers []Provider
	if err := json.Unmarshal(file, &providers); err != nil {
		return fmt.Errorf("failed to parse models json: %v", err)
	}

	AI_REGISTRY = providers
	return nil
}

func GetModelByID(id string) *AIModel {
	for _, p := range AI_REGISTRY {
		for _, m := range p.Models {
			if m.ID == id {
				return &m
			}
		}
	}
	return nil
}

func GetProviderByID(id string) *Provider {
	for _, p := range AI_REGISTRY {
		if p.ID == id {
			return &p
		}
	}
	return nil
}
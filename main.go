package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"gopkg.in/yaml.v2"
)

// Config holds the structure of the config.yaml file.
type Config struct {
	InputDir    string `yaml:"input_dir"`
	OutputDir   string `yaml:"output_dir"`
	JournalKeys string `yaml:"journal_keys"`
}

type Content struct {
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
}

type Page struct {
	ID   string  `json:"_id" yaml:"id"`
	Name string  `json:"name,omitempty" yaml:"name,omitempty"`
	Text Content `json:"text,omitempty" yaml:"text,omitempty"`
}

// Journal represents a journal document.
type Journal struct {
	ID    string `json:"_id" yaml:"id"`
	Name  string `json:"name,omitempty" yaml:"name,omitempty"`
	Pages []Page `json:"pages" yaml:"pages"`
}

func main() {
	// Find all config files matching the pattern "config-*.yaml"
	configFiles, err := filepath.Glob("config-*.yaml")
	if err != nil {
		log.Fatalf("Error finding config files: %v", err)
	}

	if len(configFiles) == 0 {
		log.Println("No config-*.yaml files found. Exiting.")
		return
	}

	for _, filePath := range configFiles {
		log.Printf("Processing config file: %s\n", filePath)
		if err := processConfig(filePath); err != nil {
			log.Printf("Failed to process %s: %v", filePath, err)
		}
	}
}

// processConfig handles the logic for a single config file
func processConfig(configPath string) error {
	// 1. Read and parse the config.yaml file.
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file %s: %w", configPath, err)
	}

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return fmt.Errorf("error unmarshaling YAML from %s: %w", configPath, err)
	}

	// 2. Open the LevelDB database.
	db, err := leveldb.OpenFile(config.InputDir, nil)
	if err != nil {
		return fmt.Errorf("error opening LevelDB at %s: %w", config.InputDir, err)
	}

	defer db.Close()

	// 3. Process the journals and their pages.
	var journals []Journal
	journalKeyList := strings.Split(config.JournalKeys, ",")

	for _, key := range journalKeyList {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		fmt.Printf("Processing journal with key: %s\n", key)

		journalValue, err := db.Get([]byte("!journal!"+key), nil)
		if err != nil {
			log.Printf("Could not find journal for key %s: %v", key, err)
			continue
		}

		var journalData map[string]interface{}
		err = json.Unmarshal(journalValue, &journalData)
		if err != nil {
			log.Printf("Could not parse JSON for journal %s: %v", key, err)
			continue
		}

		journal := Journal{
			ID: key,
			Name: func() string {
				if name, ok := journalData["name"].(string); ok {
					return name
				}
				return ""
			}(),
		}

		// Check if the journal has a pages array and iterate over it.
		if pages, ok := journalData["pages"].([]interface{}); ok {
			for _, pageID := range pages {
				if pageIDStr, ok := pageID.(string); ok {
					pageValue, err := db.Get([]byte("!journal.pages!"+key+"."+pageIDStr), nil)
					if err != nil {
						log.Printf("  - Could not find page for key %s: %v", "!journal.pages!"+key+"."+pageIDStr, err)
						continue
					}

					var pageData Page
					pageData.ID = pageIDStr
					if err := json.Unmarshal(pageValue, &pageData); err != nil {
						log.Printf("  - Could not parse JSON for page %s: %v", pageIDStr, err)
						continue
					}
					journal.Pages = append(journal.Pages, pageData)
				}
			}
		}

		journals = append(journals, journal)
	}

	// 4. Save the journals as markdown files with some yaml at the top
	for _, journal := range journals {
		journalDir := filepath.Join(config.OutputDir, journal.ID)
		if err := os.MkdirAll(journalDir, 0755); err != nil {
			log.Fatalf("Error creating journal directory: %v", err)
		}

		for _, page := range journal.Pages {
			pageFilePath := filepath.Join(journalDir, fmt.Sprintf("%s.md", page.ID))
			pageCopy := page
			pageCopy.Text = Content{} // remove text content from yaml frontmatter
			pageYaml, err := yaml.Marshal(pageCopy)
			if err != nil {
				log.Printf("Error marshaling page %s to YAML: %v", page.ID, err)
				continue
			}

			pageContent := fmt.Sprintf("---\n%s---\n\n%s", string(pageYaml), page.Text.Content)
			if err := os.WriteFile(pageFilePath, []byte(pageContent), 0644); err != nil {
				log.Printf("Error writing page file %s: %v", pageFilePath, err)
				continue
			}
		}
	}

	return nil
}

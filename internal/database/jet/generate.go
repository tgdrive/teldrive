package jet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-jet/jet/v2/generator/metadata"
	"github.com/go-jet/jet/v2/generator/postgres"
	"github.com/go-jet/jet/v2/generator/template"
	postgres2 "github.com/go-jet/jet/v2/postgres"
)

func Generate(dbURL, outputDir string) error {
	tmpOutputDir := filepath.Join(outputDir, "_tmp")
	if err := os.RemoveAll(tmpOutputDir); err != nil {
		return fmt.Errorf("clean temporary jet output: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create jet output directory: %w", err)
	}

	if err := postgres.GenerateDSN(
		dbURL,
		"teldrive",
		tmpOutputDir,
		template.Default(postgres2.Dialect).
			UseSchema(func(schema metadata.Schema) template.Schema {
				return template.DefaultSchema(schema).
					UseModel(template.DefaultModel().
						UseTable(func(table metadata.Table) template.TableModel {
							return template.DefaultTableModel(table).UseField(func(column metadata.Column) template.TableModelField {
								defaultTableModelField := template.DefaultTableModelField(column)
								if table.Name == "files" &&
									column.Name == "parts" {
									defaultTableModelField.Type = template.Type{
										Name:                  "*types.JSONB[types.Parts]",
										AdditionalImportPaths: []string{"github.com/tgdrive/teldrive/internal/database/types"}}
								}
								if table.Name == "periodic_jobs" &&
									column.Name == "args" {
									defaultTableModelField.Type = template.Type{
										Name:                  "*types.JSONB[any]",
										AdditionalImportPaths: []string{"github.com/tgdrive/teldrive/internal/database/types"}}
								}
								return defaultTableModelField
							})
						}),
					)
			}),
	); err != nil {
		return fmt.Errorf("generate jet models: %w", err)
	}

	if err := flattenGeneratedArtifacts(tmpOutputDir, outputDir); err != nil {
		return err
	}

	return nil
}

func flattenGeneratedArtifacts(tmpOutputDir, outputDir string) error {
	nestedRoot := filepath.Join(tmpOutputDir, "teldrive_jet", "teldrive")
	for _, name := range []string{"enum", "model", "table"} {
		src := filepath.Join(nestedRoot, name)
		dst := filepath.Join(outputDir, name)

		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("clean generated %s folder: %w", name, err)
		}
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("locate generated %s folder: %w", name, err)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move generated %s folder: %w", name, err)
		}
	}

	if err := os.RemoveAll(tmpOutputDir); err != nil {
		return fmt.Errorf("remove temporary jet output: %w", err)
	}

	return nil
}

package main

import (
	"log"
	"os"

	"github.com/go-jet/jet/v2/generator/metadata"
	"github.com/go-jet/jet/v2/generator/postgres"
	"github.com/go-jet/jet/v2/generator/template"
	postgres2 "github.com/go-jet/jet/v2/postgres"
)

func main() {
	err := postgres.GenerateDSN(
		os.Getenv("DB_URL"),
		"teldrive",
		"./gen",
		template.Default(postgres2.Dialect).
			UseSchema(func(schema metadata.Schema) template.Schema {
				return template.DefaultSchema(schema).
					UseModel(template.DefaultModel().
						UseTable(func(table metadata.Table) template.TableModel {
							return template.DefaultTableModel(table).
								UseField(func(column metadata.Column) template.TableModelField {
									defaultTableModelField := template.DefaultTableModelField(column)
									if table.Name == "files" &&
										column.Name == "parts" {
										defaultTableModelField.Type = template.Type{
											Name:                  "*types.JSONSlice[types.Part]",
											AdditionalImportPaths: []string{"github.com/tgdrive/teldrive/internal/database/types"}}
									}
									if table.Name == "periodic_jobs" &&
										column.Name == "args" {
										defaultTableModelField.Type = template.Type{
											Name:                  "*types.JSON",
											AdditionalImportPaths: []string{"github.com/tgdrive/teldrive/internal/database/types"}}
									}
									return defaultTableModelField
								})
						}),
					)
			}),
	)
	if err != nil {
		log.Fatal(err)
	}
}

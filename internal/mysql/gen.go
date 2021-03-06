package mysql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jinzhu/inflection"
	"vitess.io/vitess/go/vt/sqlparser"

	"github.com/kyleconroy/sqlc/internal/config"
	"github.com/kyleconroy/sqlc/internal/dinosql"
	core "github.com/kyleconroy/sqlc/internal/pg"
)

type PackageGenerator struct {
	*Schema
	config.CombinedSettings
	packageName string
}

type Result struct {
	PackageGenerator
	Queries []*Query
}

// enum value name
func enumColumnValueName(colName, value string) string {
	items := strings.Split(colName, "_")
	items = append(items, "Type", value)
	for i, v := range items {
		items[i] = strings.Title(v)
	}
	return strings.Join(items, "")
}

func getTableLastPart(tableName string) string {
	tablePart := strings.Split(tableName, "_")
	lastTablePart := ""
	if len(tablePart) > 1 {
		lastTablePart = tablePart[len(tablePart)-1]
	}
	return lastTablePart
}

// Enums generates parser-agnostic GoEnum types
func (r *Result) Enums(settings config.CombinedSettings) []dinosql.GoEnum {
	var enums []dinosql.GoEnum
	for tableName, table := range r.Schema.tables {
		lastTablePart := getTableLastPart(tableName)

		for _, col := range table {
			if col.Type.Type == "enum" {
				constants := []dinosql.GoConstant{}
				enumName, isCustomEnumName := r.enumNameFromColDef(tableName, col)
				if col.Type.NotNull == false {
					//enum default is null
					var name string
					stripped := "NULL"
					if isCustomEnumName {
						name = strings.Title(enumName) + strings.Title(stripped)
					} else {
						name = enumColumnValueName(lastTablePart+"_"+col.Name.String(), stripped)
					}
					constants = append(constants, dinosql.GoConstant{
						// Name 常量名称
						Name:  name,
						Value: stripped,
						// Type 类型名称
						Type: enumName,
					})
				}

				for _, c := range col.Type.EnumValues {
					stripped := stripInnerQuotes(c)
					var name string
					if isCustomEnumName {
						name = strings.Title(enumName) + strings.Title(stripped)
					} else {
						name = enumColumnValueName(lastTablePart+"_"+col.Name.String(), stripped)
					}
					constants = append(constants, dinosql.GoConstant{
						// TODO: maybe add the struct name call to capitalize the name here
						// Name 常量名称
						Name:  name,
						Value: stripped,
						// Type 类型名称
						Type: enumName,
					})
				}

				goEnum := dinosql.GoEnum{
					Name:      enumName,
					Comment:   "",
					Constants: constants,
					NotNull:   bool(col.Type.NotNull),
				}
				enums = append(enums, goEnum)
			}
		}
	}
	return enums
}

func stripInnerQuotes(identifier string) string {
	return strings.Replace(identifier, "'", "", 2)
}

func (pGen PackageGenerator) enumNameFromColDef(tableName string, col *sqlparser.ColumnDefinition) (string, bool) {
	tbConfig := pGen.Package.GetTable(tableName)
	colName := col.Name.String()
	if tbConfig != nil {
		customName := tbConfig.GetEnumName(colName)
		if customName != "" {
			return customName, true
		}
	}

	return fmt.Sprintf("%sType",
		dinosql.StructName(getTableLastPart(tableName)+"_"+col.Name.String(), pGen.CombinedSettings)), false
}

// Structs marshels each query into a go struct for generation
func (r *Result) Structs(settings config.CombinedSettings) []dinosql.GoStruct {
	var structs []dinosql.GoStruct
	for tableName, cols := range r.Schema.tables {
		s := dinosql.GoStruct{
			Name:  inflection.Singular(dinosql.GetStructName(tableName, settings)),
			Table: core.FQN{tableName, "", ""}, // TODO: Complete hack. Only need for equality check to see if struct can be reused between queries
		}
		tb := settings.Package.GetTable(tableName)

		for _, col := range cols {
			jsonTag := col.Name.String()
			if tb != nil {
				jsonTag = tb.GetJSONTag(jsonTag)
			}
			s.Fields = append(s.Fields, dinosql.GoField{
				Name:    dinosql.GetColumnName(col.Name.String(), tableName, settings),
				Type:    r.goTypeCol(Column{col, tableName}),
				Tags:    map[string]string{"json:": jsonTag},
				Comment: "",
			})
		}
		structs = append(structs, s)
	}
	sort.Slice(structs, func(i, j int) bool { return structs[i].Name < structs[j].Name })
	return structs
}

// GoQueries generates parser-agnostic query information for code generation
func (r *Result) GoQueries(settings config.CombinedSettings) []dinosql.GoQuery {
	structs := r.Structs(settings)

	qs := make([]dinosql.GoQuery, 0, len(r.Queries))
	for ix, query := range r.Queries {
		if query == nil {
			panic(fmt.Sprintf("query is nil on index: %v, len: %v", ix, len(r.Queries)))
		}
		if query.Name == "" {
			continue
		}
		if query.Cmd == "" {
			continue
		}
		gq := dinosql.GoQuery{
			Cmd:          query.Cmd,
			ConstantName: strings.Title(query.Name + "SQL"),
			FieldName:    dinosql.LowerTitle(query.Name) + "Stmt",
			MethodName:   query.Name,
			SourceName:   query.Filename,
			SQL:          query.SQL,
			// Comments:     query.Comments,
			Meta: query.Meta,
		}
		if len(query.Params) == 1 {
			p := query.Params[0]

			gq.Arg = dinosql.GoQueryValue{
				Name: p.GetVariableName(),
				Typ:  p.GetTypeName(),
			}
			if p.IsLikeInStmt() {
				old, newer := p.ReplaceLikeInStmt("\"", "")
				if old != "" && newer != "" {
					gq.Arg.LocalSQLQuery = strings.Replace(query.SQL, old, newer, 1)
				}
			}
		} else if len(query.Params) > 1 {
			structInfo := make([]structParams, len(query.Params))
			localSQLQuery := query.SQL
			for i := range query.Params {
				item := query.Params[i]
				originalName := item.GetVariableName()
				goTypeName := item.GetTypeName()
				old, newer := item.ReplaceLikeInStmt("\"", "arg."+strings.Title(originalName))
				if old != "" && newer != "" {
					localSQLQuery = strings.Replace(localSQLQuery, old, newer, 1)
				}
				structInfo[i] = structParams{
					originalName: originalName,
					goType:       goTypeName,
				}
			}

			gq.Arg = dinosql.GoQueryValue{
				Emit:   true,
				Name:   "arg",
				Struct: r.columnsToStruct(gq.MethodName+"Params", structInfo, settings),
			}
			if localSQLQuery != query.SQL {
				gq.Arg.LocalSQLQuery = localSQLQuery
			}
		}

		if len(query.Columns) == 1 {
			c := query.Columns[0]
			gq.Ret = dinosql.GoQueryValue{
				Name: columnName(c.ColumnDefinition, 0),
				Typ:  r.goTypeCol(c),
			}
		} else if len(query.Columns) > 1 {
			var gs *dinosql.GoStruct
			var emit bool

			for _, s := range structs {
				if len(s.Fields) != len(query.Columns) {
					continue
				}
				same := true
				for i, f := range s.Fields {
					c := query.Columns[i]
					sameName := f.Name == dinosql.GetColumnName(columnName(c.ColumnDefinition, i), s.Table.Catalog, settings)
					sameType := f.Type == r.goTypeCol(c)

					hackedFQN := core.FQN{c.Table, "", ""} // TODO: only check needed here is equality to see if struct can be reused, this type should be removed or properly used
					sameTable := s.Table.Catalog == hackedFQN.Catalog && s.Table.Schema == hackedFQN.Schema && s.Table.Rel == hackedFQN.Rel
					if !sameName || !sameType || !sameTable {
						same = false
					}
				}
				if same {
					gs = &s
					break
				}
			}

			if gs == nil {
				structInfo := make([]structParams, len(query.Columns))
				for i := range query.Columns {
					structInfo[i] = structParams{
						originalName: query.Columns[i].Name.String(),
						goType:       r.goTypeCol(query.Columns[i]),
					}
				}
				gs = r.columnsToStruct(gq.Meta.RowStructName(gq.MethodName+"Row"), structInfo, settings)
				emit = true
			}
			gq.Ret = dinosql.GoQueryValue{
				Emit:   emit,
				Name:   "i",
				Struct: gs,
			}
		}

		qs = append(qs, gq)
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].MethodName < qs[j].MethodName })
	return qs
}

type structParams struct {
	originalName string
	goType       string
}

func (r *Result) columnsToStruct(name string, items []structParams, settings config.CombinedSettings) *dinosql.GoStruct {
	gs := dinosql.GoStruct{
		Name: name,
	}
	seen := map[string]int{}
	for _, item := range items {
		name := item.originalName
		typ := item.goType
		tagName := name
		fieldName := dinosql.StructName(name, settings)
		if v := seen[name]; v > 0 {
			tagName = fmt.Sprintf("%s_%d", tagName, v+1)
			fieldName = fmt.Sprintf("%s_%d", fieldName, v+1)
		}
		gs.Fields = append(gs.Fields, dinosql.GoField{
			Name: fieldName,
			Type: typ,
			Tags: map[string]string{"json:": tagName},
		})
		seen[name]++
	}
	return &gs
}

func (pGen PackageGenerator) goTypeCol(col Column) string {
	mySQLType := col.ColumnDefinition.Type.Type
	notNull := bool(col.Type.NotNull)
	colName := col.Name.String()

	for _, oride := range pGen.Overrides {
		shouldOverride := (oride.DBType != "" && oride.DBType == mySQLType && oride.Null != notNull) ||
			(oride.ColumnName != "" && oride.ColumnName == colName && oride.Table.Rel == col.Table)
		if shouldOverride {
			return oride.GoTypeName
		}
	}
	switch t := mySQLType; {
	case "varchar" == t, "text" == t, "char" == t,
		"tinytext" == t, "mediumtext" == t, "longtext" == t:
		if col.Type.NotNull {
			return "string"
		}
		return "NullString"
	case "int" == t, "integer" == t, t == "smallint",
		"mediumint" == t, "bigint" == t, "year" == t:
		if col.Type.NotNull {
			return "int"
		}
		return "NullInt64"
	case "blob" == t, "binary" == t, "varbinary" == t, "tinyblob" == t,
		"mediumblob" == t, "longblob" == t:
		return "[]byte"
	case "float" == t, strings.HasPrefix(strings.ToLower(t), "decimal"):
		if col.Type.NotNull {
			return "float64"
		}
		return "NullFloat64"
	case "enum" == t:
		enumTypeName, _ := pGen.enumNameFromColDef(col.Table, col.ColumnDefinition)
		return enumTypeName
	case "date" == t, "timestamp" == t, "datetime" == t, "time" == t:
		if col.Type.NotNull {
			return "time.Time"
		}
		return "NullTime"
	case "boolean" == t, "bool" == t, "tinyint" == t:
		if col.Type.NotNull {
			return "bool"
		}
		return "NullBool"
	default:
		fmt.Printf("unknown MySQL type: %s\n", t)
		return "interface{}"
	}
}

func columnName(c *sqlparser.ColumnDefinition, pos int) string {
	if !c.Name.IsEmpty() {
		return c.Name.String()
	}
	return fmt.Sprintf("column_%d", pos+1)
}

func argName(name string) string {
	out := ""
	for i, p := range strings.Split(name, "_") {
		if i == 0 {
			out += strings.ToLower(p)
		} else if p == "id" {
			out += "ID"
		} else {
			out += strings.Title(p)
		}
	}
	return out
}

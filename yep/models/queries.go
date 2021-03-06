// Copyright 2016 NDP Systèmes. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/npiganeau/yep/yep/tools"
)

type SQLParams []interface{}

// Extend returns a new SQLParams with both params of this SQLParams and
// of p2 SQLParams.
func (p SQLParams) Extend(p2 SQLParams) SQLParams {
	pi := []interface{}(p)
	pi2 := []interface{}(p2)
	res := append(pi, pi2...)
	return SQLParams(res)
}

type Query struct {
	recordSet *RecordSet
	cond      *Condition
	related   []string
	limit     int
	offset    int
	groups    []string
	orders    []string
	distinct  bool
}

// sqlWhereClause returns the sql string and parameters corresponding to the
// WHERE clause of this Query
func (q *Query) sqlWhereClause() (string, SQLParams) {
	sql, args := q.conditionSQLClause(q.cond)
	if sql != "" {
		sql = "WHERE " + sql
	}
	sql, args, err := sqlx.In(sql, args...)
	if err != nil {
		tools.LogAndPanic(log, "Unable to expand 'IN' statement", "error", err, "sql", sql, "args", args)
	}
	return sql, args
}

// sqlClauses returns the sql string and parameters corresponding to the
// WHERE clause of this Condition.
func (q *Query) conditionSQLClause(c *Condition) (string, SQLParams) {
	if c.IsEmpty() {
		return "", SQLParams{}
	}
	var (
		sql  string
		args SQLParams
	)

	first := true
	for _, val := range c.params {
		vSQL, vArgs := q.condValueSQLClause(val, first)
		first = false
		sql += vSQL
		args = args.Extend(vArgs)
	}
	return sql, args
}

// sqlClause returns the sql WHERE clause for this condValue.
// If 'first' is given and true, then the sql clause is not prefixed with
// 'AND' and panics if isOr is true.
func (q *Query) condValueSQLClause(cv condValue, first ...bool) (string, SQLParams) {
	var (
		sql     string
		args    SQLParams
		isFirst bool
		adapter dbAdapter = adapters[db.DriverName()]
	)
	if len(first) > 0 {
		isFirst = first[0]
	}
	if cv.isOr && !isFirst {
		sql += "OR "
	} else if !isFirst {
		sql += "AND "
	}
	if cv.isNot {
		sql += "NOT "
	}

	if cv.isCond {
		subSQL, subArgs := q.conditionSQLClause(cv.cond)
		sql += fmt.Sprintf(`(%s) `, subSQL)
		args = args.Extend(subArgs)
	} else {
		exprs := jsonizeExpr(q.recordSet.mi, cv.exprs)
		field := q.joinedFieldExpression(exprs)
		opSql, arg := adapter.operatorSQL(cv.operator, cv.arg)
		sql += fmt.Sprintf(`%s %s `, field, opSql)
		args = append(args, arg)
	}
	return sql, args
}

// sqlLimitClause returns the sql string for the LIMIT and OFFSET clauses
// of this Query
func (q *Query) sqlLimitOffsetClause() string {
	var res string
	if q.limit > 0 {
		res = fmt.Sprintf(`LIMIT %d `, q.limit)
	}
	if q.offset > 0 {
		res += fmt.Sprintf(`OFFSET %d `, q.offset)
	}
	return res
}

// sqlOrderByClause returns the sql string for the ORDER BY clause
// of this Query
func (q *Query) sqlOrderByClause() string {
	if len(q.orders) == 0 {
		return ""
	}

	var fExprs [][]string
	directions := make([]string, len(q.orders))
	for i, order := range q.orders {
		fieldOrder := strings.Split(strings.TrimSpace(order), " ")
		oExprs := jsonizeExpr(q.recordSet.mi, strings.Split(fieldOrder[0], ExprSep))
		fExprs = append(fExprs, oExprs)
		if len(fieldOrder) > 1 {
			directions[i] = fieldOrder[1]
		}
	}
	resSlice := make([]string, len(q.orders))
	for i, field := range fExprs {
		resSlice[i] = q.joinedFieldExpression(field)
		resSlice[i] += fmt.Sprintf(" %s", directions[i])
	}
	return fmt.Sprintf("ORDER BY %s ", strings.Join(resSlice, ", "))
}

// deleteQuery returns the SQL query string and parameters to delete
// the rows pointed at by this Query object.
func (q *Query) deleteQuery() (string, SQLParams) {
	adapter := adapters[db.DriverName()]
	sql, args := q.sqlWhereClause()
	delQuery := fmt.Sprintf(`DELETE FROM %s %s`, adapter.quoteTableName(q.recordSet.mi.tableName), sql)
	return delQuery, args
}

// insertQuery returns the SQL query string and parameters to insert
// a row with the given data.
func (q *Query) insertQuery(data FieldMap) (string, SQLParams) {
	adapter := adapters[db.DriverName()]
	if len(data) == 0 {
		tools.LogAndPanic(log, "No data given for insert")
	}
	cols := make([]string, len(data))
	vals := make(SQLParams, len(data))
	var (
		i   int
		sql string
	)
	for k, v := range data {
		fi, ok := q.recordSet.mi.fields.get(k)
		if !ok {
			tools.LogAndPanic(log, "Unknown field in model", "field", k, "model", q.recordSet.mi.name)
		}
		cols[i] = fi.json
		vals[i] = v
		i++
	}
	tableName := adapter.quoteTableName(q.recordSet.mi.tableName)
	fields := strings.Join(cols, ", ")
	values := "?" + strings.Repeat(", ?", len(vals)-1)
	sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id", tableName, fields, values)
	return sql, vals
}

// countQuery returns the SQL query string and parameters to count
// the rows pointed at by this Query object.
func (q *Query) countQuery() (string, SQLParams) {
	sql, args := q.selectQuery([]string{"id"})
	delQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s) foo`, sql)
	return delQuery, args
}

// selectQuery returns the SQL query string and parameters to retrieve
// the rows pointed at by this Query object.
// fields is the list of fields to retrieve. Each field is a dot-separated
// expression pointing at the field, either as names or columns
// (e.g. 'User.Name' or 'user_id.name')
func (q *Query) selectQuery(fields []string) (string, SQLParams) {
	// Get all expressions, first given by fields
	fieldExprs := make([][]string, len(fields))
	for i, f := range fields {
		fieldExprs[i] = jsonizeExpr(q.recordSet.mi, strings.Split(f, ExprSep))
	}
	// Then given by condition
	fExprs := append(fieldExprs, q.cond.getAllExpressions(q.recordSet.mi)...)
	// Add 'order by' exprs
	for _, order := range q.orders {
		orderField := strings.Split(strings.TrimSpace(order), " ")[0]
		oExprs := jsonizeExpr(q.recordSet.mi, strings.Split(orderField, ExprSep))
		fExprs = append(fExprs, oExprs)
	}
	// Build up the query
	// Fields
	fieldsSQL := q.fieldsSQL(fieldExprs)
	// Tables
	tablesSQL := q.tablesSQL(fExprs)
	// Where clause and args
	whereSQL, args := q.sqlWhereClause()
	whereSQL += q.sqlOrderByClause()
	whereSQL += q.sqlLimitOffsetClause()
	selQuery := fmt.Sprintf(`SELECT %s FROM %s %s`, fieldsSQL, tablesSQL, whereSQL)
	return selQuery, args
}

// updateQuery returns the SQL update string and parameters to update
// the rows pointed at by this Query object with the given FieldMap.
func (q *Query) updateQuery(data FieldMap) (string, SQLParams) {
	adapter := adapters[db.DriverName()]
	if len(data) == 0 {
		tools.LogAndPanic(log, "No data given for update")
	}
	cols := make([]string, len(data))
	vals := make(SQLParams, len(data))
	var (
		i   int
		sql string
	)
	for k, v := range data {
		fi, ok := q.recordSet.mi.fields.get(k)
		if !ok {
			tools.LogAndPanic(log, "Unknown field in model", "field", k, "model", q.recordSet.mi.name)
		}
		cols[i] = fmt.Sprintf("%s = ?", fi.json)
		vals[i] = v
		i++
	}
	tableName := adapter.quoteTableName(q.recordSet.mi.tableName)
	updates := strings.Join(cols, ", ")
	whereSQL, args := q.sqlWhereClause()
	sql = fmt.Sprintf("UPDATE %s SET %s %s", tableName, updates, whereSQL)
	vals = append(vals, args...)
	return sql, vals
}

// fieldsSQL returns the SQL string for the given field expressions
// parameter must be with the following format (column names):
// [['user_id', 'name'] ['id'] ['profile_id', 'age']]
func (q *Query) fieldsSQL(fieldExprs [][]string) string {
	fStr := make([]string, len(fieldExprs))
	for i, field := range fieldExprs {
		fStr[i] = q.joinedFieldExpression(field, true)
	}
	return strings.Join(fStr, ", ")
}

// joinedFieldExpression joins the given expressions into a fields sql string
// ['profile_id' 'user_id' 'name'] => "profiles__users".name
// ['age'] => "mytable".age
// If withAlias is true, then returns fields with its alias
func (q *Query) joinedFieldExpression(exprs []string, withAlias ...bool) string {
	joins := q.generateTableJoins(exprs)
	num := len(joins)
	if len(withAlias) > 0 && withAlias[0] {
		return fmt.Sprintf("%s.%s AS %s", joins[num-1].alias, exprs[num-1], strings.Join(exprs, sqlSep))
	} else {
		return fmt.Sprintf("%s.%s", joins[num-1].alias, exprs[num-1])
	}
}

// generateTableJoins transforms a list of fields expression into a list of tableJoins
// ['user_id' 'profile_id' 'age'] => []tableJoins{CurrentTable User Profile}
func (q *Query) generateTableJoins(fieldExprs []string) []tableJoin {
	adapter := adapters[db.DriverName()]
	var joins []tableJoin

	// Create the tableJoin for the current table
	currentTableName := adapter.quoteTableName(q.recordSet.mi.tableName)
	currentTJ := tableJoin{
		tableName: currentTableName,
		joined:    false,
		alias:     currentTableName,
	}
	joins = append(joins, currentTJ)

	curMI := q.recordSet.mi
	curTJ := &currentTJ
	alias := curMI.tableName
	exprsLen := len(fieldExprs)
	for i, expr := range fieldExprs {
		fi, ok := curMI.fields.get(expr)
		if !ok {
			tools.LogAndPanic(log, "Unparsable Expression", "expr", strings.Join(fieldExprs, ExprSep))
		}
		if fi.relatedModel == nil || i == exprsLen-1 {
			// Don't create an extra join if our field is not a relation field
			// or if it is the last field of our expressions
			break
		}
		var innerJoin bool
		if fi.required {
			innerJoin = true
		}
		linkedTableName := adapter.quoteTableName(fi.relatedModel.tableName)
		alias = fmt.Sprintf("%s%s%s", alias, sqlSep, fi.relatedModel.tableName)
		nextTJ := tableJoin{
			tableName:  linkedTableName,
			joined:     true,
			innerJoin:  innerJoin,
			field:      "id",
			otherTable: curTJ,
			otherField: expr,
			alias:      adapter.quoteTableName(alias),
		}
		joins = append(joins, nextTJ)
		curMI = fi.relatedModel
		curTJ = &nextTJ
	}
	return joins
}

// tablesSQL returns the SQL string for the FROM clause of our SQL query
// including all joins if any for the given expressions.
func (q *Query) tablesSQL(fExprs [][]string) string {
	var res string
	joinsMap := make(map[string]bool)
	// Get a list of unique table joins (by alias)
	for _, f := range fExprs {
		tJoins := q.generateTableJoins(f)
		for _, j := range tJoins {
			if _, exists := joinsMap[j.alias]; !exists {
				joinsMap[j.alias] = true
				res += j.sqlString()
			}
		}
	}
	return res
}

// newQuery returns a new empty query
// If rs is given, bind this query to the given RecordSet.
func newQuery(rs ...*RecordSet) Query {
	var rset *RecordSet
	if len(rs) > 0 {
		rset = rs[0]
	}
	return Query{
		cond:      NewCondition(),
		recordSet: rset,
	}
}

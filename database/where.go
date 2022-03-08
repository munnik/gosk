package database

import (
	"fmt"
	"regexp"
	"time"
)

type WhereClause interface {
	fmt.Stringer
	Arguments() []interface{}
}

func RenumberArguments(input string) string {
	i := 0
	re := regexp.MustCompile(`(\$[0-9]+)`)
	return re.Copy().ReplaceAllStringFunc(
		input,
		func(_ string) string {
			i += 1
			return fmt.Sprintf("$%d", i)
		},
	)
}

type IntervalClause struct {
	column string
	from   time.Time
	to     time.Time
}

func NewIntervalClause(column string, from, to time.Time) IntervalClause {
	return IntervalClause{column: column, from: from, to: to}
}

func (c IntervalClause) Arguments() []interface{} {
	return []interface{}{c.from, c.to}
}

func (c IntervalClause) String() string {
	return fmt.Sprintf(`"%s" BETWEEN $1 AND $2`, c.column)
}

type AndClause struct {
	left  WhereClause
	right WhereClause
}

func NewAndClause(left, right WhereClause) AndClause {
	return AndClause{left: left, right: right}
}

func (c AndClause) Arguments() []interface{} {
	return append(c.left.Arguments(), c.right.Arguments()...)
}

func (c AndClause) String() string {
	return RenumberArguments(fmt.Sprintf("(%s AND %s)", c.left.String(), c.right.String()))
}

type OrClause struct {
	left  WhereClause
	right WhereClause
}

func NewOrClause(left, right WhereClause) OrClause {
	return OrClause{left: left, right: right}
}

func (c OrClause) Arguments() []interface{} {
	return append(c.left.Arguments(), c.right.Arguments()...)
}

func (c OrClause) String() string {
	return RenumberArguments(fmt.Sprintf("(%s OR %s)", c.left.String(), c.right.String()))
}

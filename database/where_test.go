package database_test

import (
	"time"

	. "github.com/munnik/gosk/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func checkWhere(input WhereClause, expectedQuery string, expectedArguments []interface{}) {
	Expect(input.String()).To(Equal(expectedQuery))
	Expect(input.Arguments()).To(Equal(expectedArguments))
}

var _ = Describe("Test where clauses", Ordered, func() {
	now := time.Now()
	yesterday := now.Add(-time.Hour * 24)
	tomorrow := now.Add(time.Hour * 24)

	DescribeTable("IntervalClause",
		checkWhere,
		Entry(
			"Basic",
			NewIntervalClause("time", yesterday, now),
			`"time" BETWEEN $1 AND $2`,
			[]interface{}{yesterday, now},
		),
	)
	DescribeTable("AndClause",
		checkWhere,
		Entry(
			"Correctly combines two basic where clauses",
			NewAndClause(NewIntervalClause("created", yesterday, now), NewIntervalClause("modified", now, tomorrow)),
			`("created" BETWEEN $1 AND $2 AND "modified" BETWEEN $3 AND $4)`,
			[]interface{}{yesterday, now, now, tomorrow},
		),
	)
})

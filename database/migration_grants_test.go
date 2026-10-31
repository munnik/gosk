package database_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// 20261031130100 drops the mapped_data view and creates it again around the
// uuid columns, and a recreated view starts without grants. People read
// gosk.mapped_data directly through grants made by hand, so the migration
// has to put them back, in both directions.
var _ = Describe("The uuid migration", func() {
	run := func(file string) {
		sql, err := os.ReadFile("migrations/" + file)
		Expect(err).ShouldNot(HaveOccurred())
		// Simple protocol: the file is several statements, run in one
		// implicit transaction, the way golang-migrate runs it.
		_, err = db.GetConnection().Exec(context.Background(), string(sql))
		Expect(err).ShouldNot(HaveOccurred(), file)
	}
	has := func(role, privilege string) bool {
		var ok bool
		Expect(db.GetConnection().QueryRow(context.Background(),
			`SELECT has_table_privilege($1, 'mapped_data', $2)`, role, privilege).Scan(&ok)).To(Succeed())
		return ok
	}
	// Both grants as they were: plain read access, and read access with the
	// right to pass it on.
	intact := func(when string) {
		Expect(has("mapped_data_reader", "SELECT")).To(BeTrue(), when)
		Expect(has("mapped_data_reader", "SELECT WITH GRANT OPTION")).To(BeFalse(), when)
		Expect(has("mapped_data_delegate", "SELECT WITH GRANT OPTION")).To(BeTrue(), when)
	}

	It("keeps the mapped_data view's grants when it recreates the view", func() {
		ctx := context.Background()
		for _, role := range []string{"mapped_data_reader", "mapped_data_delegate"} {
			_, err := db.GetConnection().Exec(ctx, `DROP ROLE IF EXISTS `+role)
			Expect(err).ShouldNot(HaveOccurred())
			_, err = db.GetConnection().Exec(ctx, `CREATE ROLE `+role)
			Expect(err).ShouldNot(HaveOccurred())
			DeferCleanup(func() {
				_, _ = db.GetConnection().Exec(ctx, `REVOKE ALL ON "mapped_data" FROM `+role+` CASCADE`)
				_, _ = db.GetConnection().Exec(ctx, `DROP ROLE IF EXISTS `+role)
			})
		}
		_, err := db.GetConnection().Exec(ctx, `GRANT SELECT ON "mapped_data" TO mapped_data_reader`)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = db.GetConnection().Exec(ctx, `GRANT SELECT ON "mapped_data" TO mapped_data_delegate WITH GRANT OPTION`)
		Expect(err).ShouldNot(HaveOccurred())
		intact("before")

		// The schema is past both migrations here. Down through the
		// diagnostics column and the uuid columns, then up again: each of
		// the four recreates the view.
		run("20261031130200_mapped_data_diagnostics.down.sql")
		intact("after the diagnostics down migration")
		run("20261031130100_drop_uuid_columns.down.sql")
		intact("after the uuid down migration")
		run("20261031130100_drop_uuid_columns.up.sql")
		intact("after the uuid up migration")
		run("20261031130200_mapped_data_diagnostics.up.sql")
		intact("after the diagnostics up migration")

		var columns []string
		rows, err := db.GetConnection().Query(ctx,
			`SELECT attname FROM pg_attribute WHERE attrelid = 'mapped_data'::regclass AND attnum > 0 AND NOT attisdropped`)
		Expect(err).ShouldNot(HaveOccurred())
		for rows.Next() {
			var c string
			Expect(rows.Scan(&c)).To(Succeed())
			columns = append(columns, c)
		}
		Expect(rows.Err()).ShouldNot(HaveOccurred())
		Expect(columns).To(ContainElement("diagnostics"))
		Expect(columns).NotTo(ContainElement("uuid"))
		Expect(columns).NotTo(ContainElement("transfer_uuid"))
	})
})

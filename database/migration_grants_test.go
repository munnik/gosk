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
	canRead := func(role string) bool {
		var ok bool
		Expect(db.GetConnection().QueryRow(context.Background(),
			`SELECT has_table_privilege($1, 'mapped_data', 'SELECT')`, role).Scan(&ok)).To(Succeed())
		return ok
	}

	It("keeps the mapped_data view's grants when it recreates the view", func() {
		ctx := context.Background()
		_, err := db.GetConnection().Exec(ctx, `DROP ROLE IF EXISTS "mapped_data_reader"`)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = db.GetConnection().Exec(ctx, `CREATE ROLE "mapped_data_reader"`)
		Expect(err).ShouldNot(HaveOccurred())
		DeferCleanup(func() {
			_, _ = db.GetConnection().Exec(ctx, `REVOKE ALL ON "mapped_data" FROM "mapped_data_reader"`)
			_, _ = db.GetConnection().Exec(ctx, `DROP ROLE IF EXISTS "mapped_data_reader"`)
		})
		_, err = db.GetConnection().Exec(ctx, `GRANT SELECT ON "mapped_data" TO "mapped_data_reader"`)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(canRead("mapped_data_reader")).To(BeTrue())

		// The schema is past 20261031130100 here; its down migration puts
		// the columns back and its up migration takes them out again, both
		// recreating the view.
		run("20261031130100_drop_uuid_columns.down.sql")
		Expect(canRead("mapped_data_reader")).To(BeTrue(), "after the down migration")
		run("20261031130100_drop_uuid_columns.up.sql")
		Expect(canRead("mapped_data_reader")).To(BeTrue(), "after the up migration")

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

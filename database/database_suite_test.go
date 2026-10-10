package database_test

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testDatabaseName = "gosk_test"

var (
	server *testServer
	db     *PostgresqlDatabase
)

func TestDatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database Suite")
}

// The server, the database and the schema are set up once for the whole
// suite. The suite used to upgrade before and downgrade after every single
// spec, which made every spec depend on all 46 down migrations being
// correct - a reversibility that nothing in production exercises, and that
// several of them did not have. Specs isolate themselves by clearing the
// data tables instead (see the AfterEach in postgresql_test.go), which is
// both faster and independent of the down migrations.
var _ = BeforeSuite(func() {
	var err error
	server, err = startTestServer()
	Expect(err).ShouldNot(HaveOccurred())

	Expect(server.createDatabase(testDatabaseName)).To(Succeed())

	db = NewPostgresqlDatabase(&config.PostgresqlConfig{
		URLString: server.url(testDatabaseName),
		// Flush every write immediately instead of waiting for the default
		// 100-item batch, so tests that write then read back do not have to
		// wait out the ticker below.
		BatchFlushLength:   0,
		BatchFlushInterval: 10 * time.Second,
		Timeout:            5 * time.Second,
	})

	Expect(db.UpgradeDatabase()).To(Succeed())
})

var _ = AfterSuite(func() {
	// Run the down migrations exactly once, at the end, on a schema no spec
	// needs any more. They are not free of value - 20221121162157,
	// 20221110094900, 20230203142757 and 20230419182903 were all wrong and
	// had to be fixed to get this far - but nothing in production ever runs
	// them, so they belong in one dedicated check rather than between every
	// pair of specs, which is where they used to be. This costs about 0.2s
	// for the whole suite; drop it if the down migrations ever stop being
	// worth keeping honest.
	if db != nil {
		Expect(db.DowngradeDatabase()).To(Succeed())
	}

	if server != nil {
		Expect(server.stop()).To(Succeed())
	}
})

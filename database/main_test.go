package database_test

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/pashagolub/pgxmock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	. "github.com/munnik/gosk/database"
	"github.com/munnik/gosk/nanomsg"
)

var _ = Describe("Main", func() {
	var (
		mock pgxmock.PgxConnIface
	)
	BeforeEach(func() {
		var err error
		mock, err = pgxmock.NewConn()
		if err != nil {
			Fail("Cannot create PostgreSQL mock")
		}
	})
	AfterEach(func() {
		mock.Close(context.Background())
	})
	Describe("Store raw", func() {
		Context("A valid raw message", func() {
			It("Executes the correct SQL statement", func() {
				bytesChannel := make(chan []byte)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					StoreRaw(bytesChannel, mock)
				}()

				time := time.Now().UTC()
				headers := []string{"this", "is", "a", "test"}
				payload := []byte("This is a raw test message")
				mock.ExpectExec("INSERT INTO raw_data").WithArgs(time, headers, payload).WillReturnResult(pgxmock.NewResult("INSERT", 1))
				bytesChannel <- makeRawMessage(time, headers, payload)

				close(bytesChannel)
				wg.Wait()

				if err := mock.ExpectationsWereMet(); err != nil {
					Fail(err.Error())
				}
			})
		})
		Context("Multiple valid raw messages", func() {
			It("Executes the correct SQL statement", func() {
				bytesChannel := make(chan []byte)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					StoreRaw(bytesChannel, mock)
				}()

				t := time.Now().UTC()
				h := []string{"this", "is", "a", "test"}
				p := []byte("This is a raw test message")
				mock.ExpectExec("INSERT INTO raw_data").WithArgs(t, h, p).WillReturnResult(pgxmock.NewResult("INSERT", 1))
				bytesChannel <- makeRawMessage(t, h, p)

				t = time.Now().UTC()
				h = []string{"this", "is", "a", "second", "test"}
				p = []byte("This is another raw test message")
				mock.ExpectExec("INSERT INTO raw_data").WithArgs(t, h, p).WillReturnResult(pgxmock.NewResult("INSERT", 1))
				bytesChannel <- makeRawMessage(t, h, p)

				close(bytesChannel)
				wg.Wait()

				if err := mock.ExpectationsWereMet(); err != nil {
					Fail(err.Error())
				}
			})
		})
	})
})

func makeRawMessage(time time.Time, headers []string, payload []byte) []byte {
	m := &nanomsg.RawData{
		Header: &nanomsg.Header{
			HeaderSegments: headers,
		},
		Timestamp: timestamppb.New(time),
		Payload:   payload,
	}
	toSend, _ := proto.Marshal(m)
	return toSend
}

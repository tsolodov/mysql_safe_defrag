package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/inhies/go-bytesize"

	_ "github.com/go-sql-driver/mysql"
)

var (
	dbuser          string
	dbpassword      string
	dbhost          string
	dbport          string
	dbname          string
	db_cmd          string
	threads_limit   uint64
	table_name      string
	maintenance_tbl string
	cmd             string
)

func supervisor_thread(c chan int64) {
	var conn_id int64
	var thr_cnt uint64
	var thr_state string

	println("Starting supervisor thread")

	conn_id = <-c

	db, err := sql.Open("mysql", dbuser+":"+dbpassword+"@tcp("+dbhost+":"+dbport+")/"+dbname)
	if err != nil {
		panic(err.Error())
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	time.Sleep(time.Second * 5)

	err = db.QueryRow("select upper(state)  from  information_schema.processlist where  id = ?", conn_id).Scan(&thr_state)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Thread state: %s\n", thr_state)

	err = db.QueryRow(" select count(*) as cnt from  information_schema.processlist where upper(command) not in ( upper('Connect'), upper('Sleep'), upper('Binlog Dump'))").Scan(&thr_cnt)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Initial proc count: %d\n", thr_cnt)

	for {
		err = db.QueryRow(" select count(*) as cnt from  information_schema.processlist where upper(command) not in ( upper('Connect'), upper('Sleep'), upper('Binlog Dump') )").Scan(&thr_cnt)
		if err != nil {
			panic(err.Error())
		}

		if thr_cnt > threads_limit {
			fmt.Println("ERROR: Killing worker thread due waiting proc count")
			_, err := db.Query("KILL ?", conn_id)
			if err != nil {
				panic(err.Error())
			}

		}

		for i := 1; i < 5; i++ {
			err = db.QueryRow("select upper(state)  from  information_schema.processlist where  id = ?", conn_id).Scan(&thr_state)
			if err != nil {
				panic(err.Error())
			}
			fmt.Printf("\rworker thread state: %s , num of active threads: %v ", thr_state, thr_cnt)
			time.Sleep(time.Second * 1)
			fmt.Printf(".")

		}

	}

}

func worker_thread(c chan int64) {
	var conn_id int64
	// var conn_new int64
	var sql_bin_log int
	var db_size_old float64
	var db_size_new float64
	var db_space_reclaimed float64

	println("Starting worker thread")

	db_space_reclaimed = 0.0

	db, err := sql.Open("mysql", dbuser+":"+dbpassword+"@tcp("+dbhost+":"+dbport+")/"+dbname)
	if err != nil {
		panic(err.Error())
	}

	_, err = db.Exec("set session sql_log_bin = 0")
	if err != nil {
		panic(err.Error())
	}

	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	err = tx.QueryRow("select CONNECTION_ID()").Scan(&conn_id)
	if err != nil {
		panic(err.Error())
	}
	err = tx.QueryRow("select @@sql_log_bin").Scan(&sql_bin_log)
	if err != nil {
		panic(err.Error())
	}
	if sql_bin_log != 0 {
		tx.Rollback()
		fmt.Printf("sql_log_bin = %d", sql_bin_log)
		panic("recived sql_log_bin = 1")
	}

	c <- conn_id

	fmt.Printf("Received connection ID from worker thread: %d\n", conn_id)
	for _, table_name := range strings.Split(maintenance_tbl, ",") {

		fmt.Printf("Working on %s table\n", table_name)
		err = tx.QueryRow(" select INDEX_LENGTH + DATA_LENGTH from information_schema.tables where TABLE_SCHEMA = ? AND TABLE_NAME = ?", dbname, table_name).Scan(&db_size_old)
		if err != nil {
			panic(err.Error())
		}
		b := bytesize.New(db_size_old)

		fmt.Printf("TBL size before: %s\n", b)
		cmd = fmt.Sprintf(db_cmd, table_name)
		fmt.Printf("Going to execute command: '%s'\n", cmd)
		start := time.Now()

		_, err = tx.Exec("/* DBA activity: " + os.Getenv("LOGNAME") + " */" + cmd)
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("%s executed successfully\n", cmd)
		duration := time.Since(start)
		fmt.Printf("Took %s to execute\n", duration)
		// }
		err = tx.QueryRow(" select INDEX_LENGTH + DATA_LENGTH from information_schema.tables where TABLE_SCHEMA = ? AND TABLE_NAME = ?", dbname, table_name).Scan(&db_size_new)
		if err != nil {
			panic(err.Error())
		}
		b = bytesize.New(db_size_new)
		fmt.Printf("TBL size after: %s\n", b)
		b = bytesize.New(db_size_old - db_size_new)
		db_space_reclaimed = db_space_reclaimed + db_size_old - db_size_new

		fmt.Printf("Reclaimed space %s, total: %s\n", b, bytesize.New(db_space_reclaimed))

	}
	defer db.Close()

	tx.Commit()

	b := bytesize.New(db_space_reclaimed)
	fmt.Printf("TOTAL reclaimed space: %s\n", b)
}

func main() {
	println("Starting main thread...")

	dbuser = os.Getenv("DB_USER")
	dbpassword = os.Getenv("DB_PASSWORD")
	dbhost = os.Getenv("DB_HOST")
	dbname = os.Getenv("DB_NAME")
	db_cmd = os.Getenv("DB_CMD")
	maintenance_tbl = os.Getenv("DEFRAG_TABLES")
	table_name = maintenance_tbl
	dbport = "3306"
	threads_limit = 10

	if os.Getenv("DB_PORT") != "" {
		_, err := strconv.ParseUint(os.Getenv("DB_PORT"), 10, 64)
		if err != nil {
			panic(err.Error())
		}
		dbport = os.Getenv("DB_PORT")

	}

	if os.Getenv("THREADS_LIMIT") != "" {
		var err error
		threads_limit, err = strconv.ParseUint(os.Getenv("THREADS_LIMIT"), 10, 64)
		if err != nil {
			panic(err.Error())
		}
	}

	if dbuser == "" {
		panic("Please set db user via DB_USER")
	}

	if dbpassword == "" {
		panic("Please set db user via DB_PASSWORD")
	}

	if dbhost == "" {
		panic("Please set db host via DB_HOST")
	}

	if dbname == "" {
		panic("Please set db name via DB_NAME")

	}

	if db_cmd == "" {
		panic("Please set cmd name via DB_CMD")

	}
	if maintenance_tbl == "" {
		panic("Please set defrag tables via DEFRAG_TABLES")

	}
	println("OK")

	conn := make(chan int64)

	fmt.Printf("Building connection to %s:*****@%s/%s\n", dbuser, dbhost, dbname)
	go supervisor_thread(conn)

	worker_thread(conn)

}

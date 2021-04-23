# mysql_safe_defrag

Safe Innodb table defrag. The tool creates 2 connections to MySQL service: worker and supervisor. Supervisor monitors processlist and if it finds spike in active threads count, it will kill worker connection(Safety is first). Main purpose:  avoid wasting of time during ong-running alter commands and let it go on it's own.


It does alter with sql_bin_log = 0, I implemented it for a reason. When you do alter table with binary log enabled, the long-running alter will be replicated, and if you have complex replication topology, like M1->S1->S2->S3, you will be waiting for time_of_alter*(num_of_replicas_in_chain+1). If alters takes 2h, for this example it would be 2h*4.(approximatelly, it dedpends on multiple factors: how the ervers are loaded/HW/etc...). I prefer to run this tool in parallel for each DB server in replication chain.

## Disclaimner

USE AT YOUR OWN RISK. FOR ME THIS TOOL PERFECTLY WORKS, BUT YOU NEED TO TEST IT ON YOUR SETUP BEFORE RUNNING IN PRODUCTION.

## Usage
Logic is controlled by environment variables:

DB_PORT - default is 3306

DB_NAME - database name

DB_HOST - database host

DB_USER - user

DB_PASSWORD - password

THREADS_LIMIT - default is 10. Number of threads in process list other than Connect, Sleep, Binlog Dump states. Once it's reaches it, I assume there is some problem and it is better to kill alter and stop the alter table process. The problem might be related to other activity and not nessesary caused by alter. 

DEFRAG_TABLES - comma separated list: t1,t2,t3

DB_CMD - MySQL cmd. Example: 'alter table %s engine=innodb,algorithm=inplace'



DB_PORT=3307 DEFRAG_TABLES=table1,table2,table3 DB_CMD='alter table %s engine=innodb,algorithm=inplace' DB_NAME=dbname DB_HOST=dbhost.example.com DB_USER=USER DB_PASSWORD=XXXXX ./mysql_safe_defrag

This command will run sequentially:

alter table table1 engine=innodb,algorithm=inplace
alter table table2 engine=innodb,algorithm=inplace
alter table table3 engine=innodb,algorithm=inplace


### Example

*Get tables greater than 500MB and order by size*

```
cat<<EOF | mysql -h dbhost.example.com |tr "\n" ","
select table_name from information_schema.tables where table_schema = 'dbname' AND INDEX_LENGTH+DATA_LENGTH > 1024*1024*500 AND ENGINE = 'InnoDB' order by INDEX_LENGTH+DATA_LENGTH;
EOF
```

```
DEFRAG_TABLES=table1 DB_CMD='alter table %s engine=innodb,algorithm=inplace' DB_NAME=dbname DB_HOST=dbhost.example.com DB_USER=USER DB_PASSWORD=XXXXX ./mysql_safe_defrag


Starting main thread...
OK
Building connection to USER:*****@dbhost.example.com/dbname
Starting worker thread
Starting supervisor thread
Received connection ID from worker thread: 753189
Working on table1 table
TBL size before: 150.80MB
Going to execute command: 'alter table table1 engine=innodb'
Thread state: ALTERING TABLE
Initial proc count: 2
worker thread state: ALTERING TABLE , num of active threads: 2 alter table table1 engine=innodb executed successfully
Took 27.912115312s to execute
TBL size after: 150.80MB
Reclaimed space 0.00B, total: 0.00B
TOTAL reclaimed space: 0.00B
```

## Building

git clone git@github.com:tsolodov/mysql_safe_defrag.git

cd mysql_safe_defrag;

go build -o mysql_safe_defrag main.go

chmod +x mysql_safe_defrag

## Reporting bugs/feature requests

Feel free to open bugs in github, If I have time and interest, I will try to fix it.



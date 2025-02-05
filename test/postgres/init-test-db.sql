CREATE USER test_user WITH PASSWORD 'password';
CREATE DATABASE test_rollupsdb OWNER test_user;
GRANT CONNECT ON DATABASE test_rollupsdb TO test_user;
GRANT CREATE ON DATABASE test_rollupsdb TO test_user;
GRANT TEMP ON DATABASE test_rollupsdb TO test_user;

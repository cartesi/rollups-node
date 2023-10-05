# Rollups data

This crate generates the PostgreSQL database schema used to store the rollups data: 
all of its inputs, notices, vouchers, reports and proofs.

## Running PostgreSQL locally

If you want to run the PostgreSQL in your machine, you can run the following docker command:

```sh
docker run --rm --name test-postgres -e POSTGRES_PASSWORD=pw -p 5432:5432 -d postgres:13
```

## Setup diesel

The database migration requires [diesel](https://diesel.rs/).
Before installing diesel, you need to install the PostgreSQL development library.
In Ubuntu, run the following command:

```sh
sudo apt install libpq-dev
```

Or if you are using MacOS:

```sh
brew install libpq &&
echo 'export PATH="/opt/homebrew/opt/libpq/bin:$PATH"' >> ~/.zshrc
```

To install diesel run the following command:

```sh
cargo install diesel_cli --no-default-features --features postgres
```

Then, setup the environment variable DATABASE\_URL referencing to the local PostgreSQL.

```sh
echo "export DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres" > .env
```

## Perform database migrations

Follow the commands below to perform the migration.
This procedure creates the tables in PostgreSQL and also generates the file `src/schema.rs`.

```sh
diesel migration run
```

## Modifying the database schema

To modify the database schema, you should edit the files in the `migration` dir.
For more details, please follow the instructions on [the diesel site](https://diesel.rs).

## Test

To run the automated tests, run the following command:

```sh
cargo test
```

### Manual tests

If you want to fiddle with the database, you can populate it by running:

```sh
psql -h localhost -U postgres -d postgres -a -f util/populate.sql
```

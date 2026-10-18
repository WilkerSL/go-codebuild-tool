package gen

import "strings"

type ciView struct {
	Services        string
	TestDatabaseURL string
	Setup           string
}

func indentLines(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func newCIView(dialectName string) ciView {
	switch dialectName {
	case "postgres":
		return ciView{
			Services: indentLines(`postgres:
  image: postgres:17
  env:
    POSTGRES_USER: app
    POSTGRES_PASSWORD: app
    POSTGRES_DB: app
  ports:
    - 5432:5432
  options: >-
    --health-cmd "pg_isready -U app"
    --health-interval 5s
    --health-timeout 5s
    --health-retries 30`, "      "),
			TestDatabaseURL: "postgres://app:app@localhost:5432/app?sslmode=disable",
		}
	case "sqlserver":
		return ciView{
			Services: indentLines(`sqlserver:
  image: mcr.microsoft.com/mssql/server:2022-latest
  env:
    ACCEPT_EULA: "Y"
    MSSQL_SA_PASSWORD: Test_Passw0rd
  ports:
    - 1433:1433
  options: >-
    --name sqlserver
    --health-cmd "/opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P Test_Passw0rd -C -Q 'SELECT 1'"
    --health-interval 10s
    --health-timeout 5s
    --health-retries 30`, "      "),
			TestDatabaseURL: "sqlserver://sa:Test_Passw0rd@localhost:1433?database=app&encrypt=disable",
			Setup: indentLines(`- name: Create the test database
  run: docker exec sqlserver /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P Test_Passw0rd -C -Q "CREATE DATABASE app"`, "      "),
		}
	case "mysql":
		return ciView{
			Services: indentLines(`mysql:
  image: mysql:8.4
  env:
    MYSQL_ROOT_PASSWORD: root
    MYSQL_DATABASE: app
    MYSQL_USER: app
    MYSQL_PASSWORD: app
  ports:
    - 3306:3306
  options: >-
    --health-cmd "mysqladmin ping -h 127.0.0.1 -uroot -proot"
    --health-interval 5s
    --health-timeout 5s
    --health-retries 30`, "      "),
			TestDatabaseURL: "app:app@tcp(localhost:3306)/app",
		}
	}
	return ciView{}
}

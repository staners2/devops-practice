import os
import sys

import psycopg
from psycopg import sql


def main():
    if len(sys.argv) != 2 or not os.environ.get("DEVOPS_PASSWORD"):
        print("post-bootstrap requires a Patroni connection argument and DEVOPS_PASSWORD", file=sys.stderr)
        return 1

    role = "devops"
    database = "devops_practice"
    try:
        with psycopg.connect(
            sys.argv[1],
            connect_timeout=10,
            autocommit=True,
            # PostgreSQL utility statements need client-side parameter binding.
            cursor_factory=psycopg.ClientCursor,
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", (role,))
                if cursor.fetchone() is None:
                    cursor.execute(sql.SQL("CREATE ROLE {}").format(sql.Identifier(role)))
                cursor.execute(
                    sql.SQL("ALTER ROLE {} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %s").format(
                        sql.Identifier(role)
                    ),
                    (os.environ["DEVOPS_PASSWORD"],),
                )
                cursor.execute("SELECT 1 FROM pg_database WHERE datname = %s", (database,))
                if cursor.fetchone() is None:
                    cursor.execute(
                        sql.SQL("CREATE DATABASE {} OWNER {} TEMPLATE template0").format(
                            sql.Identifier(database), sql.Identifier(role)
                        )
                    )
    except psycopg.Error as error:
        # Driver exceptions may contain credentials or the SQL password literal.
        print(f"post-bootstrap failed: {type(error).__name__}, SQLSTATE {error.sqlstate or 'unknown'}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

import contextlib
import importlib.util
import io
from pathlib import Path
import unittest
from unittest.mock import MagicMock, patch

import psycopg


spec = importlib.util.spec_from_file_location("post_bootstrap", Path(__file__).parents[1] / "post-bootstrap.py")
bootstrap = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bootstrap)


class PostBootstrapTests(unittest.TestCase):
    def test_creates_role_and_database_with_quoted_identifiers_and_password_parameter(self):
        password = "a'password; DROP ROLE devops; --"
        connection = MagicMock()
        cursor = connection.cursor.return_value.__enter__.return_value
        cursor.fetchone.side_effect = [None, None]
        with patch.object(bootstrap.sys, "argv", ["post-bootstrap.py", "postgresql://postgres@localhost/postgres"]), \
             patch.dict(bootstrap.os.environ, {"DEVOPS_PASSWORD": password}), \
             patch.object(bootstrap.psycopg, "connect") as connect:
            connect.return_value.__enter__.return_value = connection
            self.assertEqual(bootstrap.main(), 0)
        connect.assert_called_once_with(
            "postgresql://postgres@localhost/postgres", connect_timeout=10,
            autocommit=True, cursor_factory=psycopg.ClientCursor,
        )
        calls = cursor.execute.call_args_list
        self.assertEqual(calls[1].args[0].as_string(), 'CREATE ROLE "devops"')
        self.assertIn('ALTER ROLE "devops"', calls[2].args[0].as_string())
        self.assertIn("PASSWORD %s", calls[2].args[0].as_string())
        self.assertEqual(calls[2].args[1], (password,))
        self.assertEqual(
            calls[4].args[0].as_string(),
            'CREATE DATABASE "devops_practice" OWNER "devops" TEMPLATE template0',
        )

    def test_existing_role_and_database_are_not_recreated(self):
        connection = MagicMock()
        cursor = connection.cursor.return_value.__enter__.return_value
        cursor.fetchone.side_effect = [(1,), (1,)]
        with patch.object(bootstrap.sys, "argv", ["post-bootstrap.py", "postgresql://postgres@localhost/postgres"]), \
             patch.dict(bootstrap.os.environ, {"DEVOPS_PASSWORD": "test-password"}), \
             patch.object(bootstrap.psycopg, "connect") as connect:
            connect.return_value.__enter__.return_value = connection
            self.assertEqual(bootstrap.main(), 0)
        self.assertEqual(cursor.execute.call_count, 3)
        self.assertIn("ALTER ROLE", cursor.execute.call_args_list[1].args[0].as_string())

    def test_driver_error_does_not_expose_credentials(self):
        password = "password-that-must-not-be-logged"
        output = io.StringIO()
        with patch.object(bootstrap.sys, "argv", ["post-bootstrap.py", "postgresql://postgres@localhost/postgres"]), \
             patch.dict(bootstrap.os.environ, {"DEVOPS_PASSWORD": password}), \
             patch.object(bootstrap.psycopg, "connect", side_effect=psycopg.ProgrammingError(password)), \
             contextlib.redirect_stderr(output):
            self.assertEqual(bootstrap.main(), 1)
        self.assertIn("ProgrammingError", output.getvalue())
        self.assertNotIn(password, output.getvalue())

    def test_missing_password_fails_before_connecting(self):
        with patch.object(bootstrap.sys, "argv", ["post-bootstrap.py", "postgresql://postgres@localhost/postgres"]), \
             patch.dict(bootstrap.os.environ, {"DEVOPS_PASSWORD": ""}), \
             patch.object(bootstrap.psycopg, "connect") as connect, \
             contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(bootstrap.main(), 1)
        connect.assert_not_called()


if __name__ == "__main__":
    unittest.main()

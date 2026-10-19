import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class InitializerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.repository = Path(__file__).resolve().parents[3]
        cls.temporary = tempfile.TemporaryDirectory(prefix='devops-levels-')
        cls.course = Path(cls.temporary.name) / 'course with spaces'
        source = cls.repository / '02-interviews-and-practice' / 'artifacts'
        cls.artifacts = cls.course / '02-interviews-and-practice' / 'artifacts'
        shutil.copytree(
            source, cls.artifacts,
            ignore=shutil.ignore_patterns(
                '.env', '*.pem', '*.key', 'node_modules', '.venv',
                '.terraform', '.ansible', '__pycache__', '.DS_Store', 'tests',
            ),
        )
        cls.profile_file = cls.artifacts / 'profiles.json'
        cls.profiles = json.loads(cls.profile_file.read_text())
        for task in cls.profiles:
            destination = cls.course / task
            destination.mkdir(parents=True)
            shutil.copy2(cls.repository / task / 'README.md', destination / 'README.md')
        (cls.artifacts / 'common' / '.env').write_text('LOCAL_ONLY=do-not-copy\n')
        cls.script = cls.artifacts / 'init.sh'

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def initialize(self, *arguments):
        return subprocess.run(
            ['bash', str(self.script), *arguments],
            cwd=self.temporary.name, capture_output=True, text=True,
        )

    def test_all_profiles_and_refusal_to_overwrite(self):
        for task_path, profile in self.profiles.items():
            with self.subTest(task=task_path):
                level, topic, exercise = task_path.split('/')
                arguments = (level[:2], topic[:2], exercise[:2])
                result = self.initialize(*arguments)
                self.assertEqual(result.returncode, 0, result.stderr)
                workspace = self.course / task_path / 'workspace'
                try:
                    self.assertTrue((workspace / 'TASK.md').is_file())
                    self.assertIn('../README.md', (workspace / 'TASK.md').read_text())
                    self.assertFalse((workspace / '.env').exists())
                    if profile == '06':
                        self.assertTrue((workspace / 'deploy/kafka-lab/compose.yaml').is_file())
                    if profile == '00':
                        self.assertTrue((workspace / 'Dockerfile').is_file())
                        self.assertTrue((workspace / 'deploy/compose/compose.yaml').is_file())
                    marker = workspace / 'learner.txt'
                    marker.write_text('keep my work')
                    repeated = self.initialize(*arguments)
                    self.assertNotEqual(repeated.returncode, 0)
                    self.assertIn('already exists', repeated.stderr)
                    self.assertEqual(marker.read_text(), 'keep my work')
                    self.assertFalse(list(workspace.parent.glob('.init-*')))
                finally:
                    shutil.rmtree(workspace)

    def test_theory_level_and_invalid_identifiers_are_rejected(self):
        for arguments in [
            ('01', '06', '01'), ('04', '06', '01'), ('02', '../06', '01'),
            ('02', '6', '01'), ('02', '06', '1'), ('02', '06', '99'),
            ('03', '02', '01'),
        ]:
            with self.subTest(arguments=arguments):
                result = self.initialize(*arguments)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('Traceback', result.stderr)

    def test_missing_arguments_show_usage(self):
        result = self.initialize('02', '06')
        self.assertEqual(result.returncode, 2)
        self.assertIn('Usage:', result.stderr)

    def test_existing_workspace_symlink_is_not_followed(self):
        task = self.course / '02-interviews-and-practice/06-kubernetes/01-basic'
        workspace = task / 'workspace'
        workspace.symlink_to(self.course / 'missing-target')
        try:
            result = self.initialize('02', '06', '01')
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(workspace.is_symlink())
            self.assertFalse((self.course / 'missing-target').exists())
        finally:
            workspace.unlink()

    def test_missing_profile_has_a_readable_error(self):
        original = self.profile_file.read_text()
        profiles = json.loads(original)
        del profiles['02-interviews-and-practice/06-kubernetes/01-basic']
        self.profile_file.write_text(json.dumps(profiles))
        try:
            result = self.initialize('02', '06', '01')
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('No initialization profile', result.stderr)
            self.assertNotIn('Traceback', result.stderr)
        finally:
            self.profile_file.write_text(original)


if __name__ == '__main__':
    unittest.main()

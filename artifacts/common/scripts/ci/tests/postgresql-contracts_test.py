import unittest
from pathlib import Path

import yaml


class PostgreSQLContractsTests(unittest.TestCase):
    def test_required_security_job_exists_in_merge_request_pipelines(self):
        ci = yaml.safe_load(Path(".gitlab-ci.yml").read_text())
        rules = ci["gitleaks"].get("rules", [])
        self.assertTrue(any(rule.get("when") == "on_success" and "if" not in rule
                            for rule in rules), "required security job must be explicitly present in every allowed pipeline")
        self.assertIn("gitleaks", [need["job"] for need in ci["build-patroni"]["needs"]])

    def test_restore_metrics_have_release_scoped_service_label(self):
        config = yaml.safe_load(Path("deploy/monitoring/scrape/vmagent.yaml").read_text())
        application = next(job for job in config["scrape_configs"] if job["job_name"] == "application")
        rules = application["relabel_configs"]
        self.assertTrue(any(rule.get("source_labels") == ["__meta_kubernetes_pod_label_app_kubernetes_io_name",
                                                         "__meta_kubernetes_pod_label_app_kubernetes_io_instance"]
                            and rule.get("regex") == "devops-practice;(.+)"
                            and rule.get("target_label") == "service" for rule in rules))

    def test_both_architectures_are_selected_before_scan(self):
        ci = yaml.safe_load(Path(".gitlab-ci.yml").read_text())
        for job in ("scan-patroni", "sbom-patroni"):
            self.assertEqual([need["job"] for need in ci[job]["needs"]], ["split-patroni"])
            commands = "\n".join(ci[job]["script"])
            for architecture in ("amd64", "arm64"):
                self.assertIn("patroni-" + architecture + ".tar", commands)
            self.assertNotIn("patroni-image.oci.tar", commands)
        publish = ci["publish-patroni"]
        self.assertIn("scan-patroni", [need["job"] for need in publish["needs"]])
        self.assertIn("sbom-patroni", [need["job"] for need in publish["needs"]])


if __name__ == "__main__":
    unittest.main()

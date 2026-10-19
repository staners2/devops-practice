package migrations

import "testing"

func TestListPreservesInitialAndOrdersVersions(t *testing.T) {
	versions, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) < 2 || versions[0].Version != 1 || versions[1].Version != 2 {
		t.Fatalf("unexpected migration sequence: %#v", versions)
	}
	for i, migration := range versions {
		if migration.SQL == "" || (i > 0 && migration.Version <= versions[i-1].Version) {
			t.Fatal("migrations must have SQL and unique ascending versions")
		}
	}
}

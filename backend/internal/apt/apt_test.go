package apt

import "testing"

func TestParseDpkgQuery(t *testing.T) {
	out := "dreamhub\t0.3.0\tweb\tDreamHub web interface\n" +
		"dropbear\t2022.83\tconsole/network\tLightweight SSH server\n" +
		"no-description\t1.0\tbase\t\n"
	pkgs := ParseDpkgQuery([]byte(out))
	if len(pkgs) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(pkgs), pkgs)
	}
	if pkgs[0] != (Package{Name: "dreamhub", Version: "0.3.0", Category: "web", Description: "DreamHub web interface"}) {
		t.Errorf("pkgs[0] = %+v", pkgs[0])
	}
	if pkgs[2].Name != "no-description" || pkgs[2].Description != "" {
		t.Errorf("pkgs[2] = %+v", pkgs[2])
	}
}

func TestParseAptSimulate(t *testing.T) {
	out := `Reading package lists...
Building dependency tree...
The following packages will be upgraded:
  oscam-cycecplus
1 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
Inst oscam-cycecplus [11742] (11760 dreamhub-feed [arm64])
Conf oscam-cycecplus (11760 dreamhub-feed [arm64])
`
	versions := ParseAptSimulate([]byte(out))
	if versions["oscam-cycecplus"] != "11760" {
		t.Fatalf("versions = %v", versions)
	}
	if len(versions) != 1 {
		t.Errorf("len(versions) = %d, want 1", len(versions))
	}
}

func TestApplyUpgradable(t *testing.T) {
	pkgs := []Package{{Name: "a"}, {Name: "b"}}
	applyUpgradable(pkgs, map[string]string{"b": "2.0"})
	if pkgs[0].Upgradable {
		t.Errorf("a should not be upgradable")
	}
	if !pkgs[1].Upgradable || pkgs[1].AvailableVersion != "2.0" {
		t.Errorf("b = %+v", pkgs[1])
	}
}

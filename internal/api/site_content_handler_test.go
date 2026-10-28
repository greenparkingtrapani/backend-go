package api

import "testing"

func TestSiteContentScope(t *testing.T) {
	if !validSiteContentScope("it", "HeroSection") {
		t.Fatal("expected valid scope")
	}
	for _, c := range [][2]string{{"fr", "HeroSection"}, {"it", "Unknown"}, {"", "SEO"}} {
		if validSiteContentScope(c[0], c[1]) {
			t.Fatalf("unexpected valid scope: %v", c)
		}
	}
}

func TestSiteContentValidation(t *testing.T) {
	valid := siteContentSave{Language: "it", Section: "Header", Fields: map[string]string{"Header.title": "Titolo"}}
	if err := validateSiteContentSave(valid); err != nil {
		t.Fatal(err)
	}
	cases := []siteContentSave{
		{Language: "it", Section: "Header", Fields: map[string]string{}},
		{Language: "it", Section: "Header", Fields: map[string]string{"Footer.title": "wrong section"}},
		{Language: "de", Section: "Header", Fields: map[string]string{"Header.title": "no"}},
	}
	for _, c := range cases {
		if validateSiteContentSave(c) == nil {
			t.Fatalf("expected validation error: %#v", c)
		}
	}
}

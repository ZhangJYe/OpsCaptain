package evalharness

import "fmt"

func ValidateRegressionCorpus(manifest *Manifest) error {
	if manifest == nil || manifest.RegressionCorpus.MinTotal == 0 {
		return nil
	}
	if manifest.DatasetRole != DatasetRegression || manifest.Profile != ProfileDeterministic {
		return fmt.Errorf("regression corpus constraints require regression + deterministic")
	}
	total := 0
	for _, suite := range manifest.Suites {
		if !suite.Enabled {
			continue
		}
		cases, _, err := LoadCases(manifest.SourcePath, suite)
		if err != nil {
			return err
		}
		total += len(cases)
		if minimum := manifest.RegressionCorpus.SuiteMinimums[suite.Name]; len(cases) < minimum {
			return fmt.Errorf("suite %s requires at least %d regression cases, got %d", suite.Name, minimum, len(cases))
		}
		required := make(map[string]bool, len(manifest.RegressionCorpus.RequiredTags[suite.Name]))
		for _, tag := range manifest.RegressionCorpus.RequiredTags[suite.Name] {
			required[tag] = true
		}
		for _, evalCase := range cases {
			for _, tag := range evalCase.Tags {
				delete(required, tag)
			}
		}
		for tag := range required {
			return fmt.Errorf("suite %s is missing required regression tag %q", suite.Name, tag)
		}
	}
	if total < manifest.RegressionCorpus.MinTotal {
		return fmt.Errorf("regression corpus requires at least %d cases, got %d", manifest.RegressionCorpus.MinTotal, total)
	}
	return nil
}

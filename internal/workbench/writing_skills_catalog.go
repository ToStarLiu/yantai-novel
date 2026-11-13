package workbench

import _ "embed"

//go:embed data/writing-skill-catalog.json
var writingSkillCatalog []byte

func WritingSkillCatalog() []byte { return append([]byte(nil), writingSkillCatalog...) }

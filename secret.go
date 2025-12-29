package main

type Secret struct {
	SourceMetadata        SourceMetadata `json:"SourceMetadata"`
	SourceID              int            `json:"SourceID"`
	SourceType            int            `json:"SourceType"`
	SourceName            string         `json:"SourceName"`
	DetectorType          int            `json:"DetectorType"`
	DetectorName          string         `json:"DetectorName"`
	DetectorDescription   string         `json:"DetectorDescription"`
	DecoderName           string         `json:"DecoderName"`
	Verified              bool           `json:"Verified"`
	VerificationFromCache bool           `json:"VerificationFromCache"`
	VerificationError     string         `json:"VerificationError,omitempty"`
	Raw                   string         `json:"Raw"`
	RawV2                 string         `json:"RawV2,omitempty"`
	Redacted              string         `json:"Redacted"`
	ExtraData             any            `json:"ExtraData,omitempty"`
	StructuredData        any            `json:"StructuredData,omitempty"`
}

type SourceMetadata struct {
	Data SourceData `json:"Data"`
}

type SourceData struct {
	Github *GithubMetadata `json:"Github,omitempty"`
}

type GithubMetadata struct {
	Link                string `json:"link"`
	Repository          string `json:"repository"`
	Commit              string `json:"commit"`
	Email               string `json:"email"`
	File                string `json:"file"`
	Timestamp           string `json:"timestamp"`
	Line                int    `json:"line"`
	RepositoryLocalPath string `json:"repository_local_path"`
}

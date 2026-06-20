// Package qoder implements the COSY signing scheme and WAF-bypass body
// encoding required by Qoder's inference API at api3.qoder.sh.
//
// Every signed request carries:
//   - an AES-128-CBC payload of the user info, the AES key wrapped in RSA
//   - an MD5 signature over payload || cosyKey || timestamp || body || sigPath
//   - the body's MD5 hash + length so the server can validate integrity
//   - ~17 Cosy-* / X-* headers fingerprinting the client
package qoder

// Endpoints.
const (
	// ChatURLEncoded is the COSY-signed inference endpoint with Encode=1 so
	// we can ship the body through the WAF-bypass encoder.
	ChatURLEncoded = "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation" +
		"?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"

	// ChatURL is the same endpoint without body encoding.
	ChatURL = "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation" +
		"?FetchKeys=llm_model_result&AgentId=agent_common"

	// ModelListURL returns the live model catalog (COSY-signed GET).
	ModelListURL = "https://api3.qoder.sh/algo/api/v2/model/list"
)

// COSY header fingerprint constants. These are not arbitrary — the upstream
// signature validation matches them against the values used at signing time.
const (
	IDEVersion   = "1.0.0"
	ClientType   = "5"
	DataPolicy   = "disagree"
	LoginVersion = "v2"
	MachineOS    = "x86_64_windows"
	MachineType  = "5"
)

// RSAPublicKeyPEM is the RSA public key for COSY encryption (extracted from
// Qoder IDE v0.9). Matches the CLIProxyAPIPlus branch and live qodercli
// traffic.
const RSAPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

// ModelKeys enumerates the canonical Qoder model identifiers.
var ModelKeys = []string{
	// Tier models
	"auto", "ultimate", "performance", "efficient", "lite",
	// Frontier models
	"qmodel", "qmodel_latest",
	"dmodel", "dfmodel",
	"gm51model", "kmodel", "mmodel",
}

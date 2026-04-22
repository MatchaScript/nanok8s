package kubeconfig

// Minimal kubeconfig schema, sufficient for nanok8s-generated files.
// Field names (via json tags) follow the wire format kubectl consumes.

type kubeconfigFile struct {
	APIVersion     string          `json:"apiVersion"`
	Kind           string          `json:"kind"`
	Clusters       []namedCluster  `json:"clusters"`
	Contexts       []namedContext  `json:"contexts"`
	Users          []namedUser     `json:"users"`
	CurrentContext string          `json:"current-context"`
}

type namedCluster struct {
	Name    string        `json:"name"`
	Cluster clusterFields `json:"cluster"`
}

type clusterFields struct {
	Server                   string `json:"server"`
	CertificateAuthorityData string `json:"certificate-authority-data"`
}

type namedContext struct {
	Name    string        `json:"name"`
	Context contextFields `json:"context"`
}

type contextFields struct {
	Cluster string `json:"cluster"`
	User    string `json:"user"`
}

type namedUser struct {
	Name string     `json:"name"`
	User userFields `json:"user"`
}

type userFields struct {
	ClientCertificateData string `json:"client-certificate-data"`
	ClientKeyData         string `json:"client-key-data"`
}

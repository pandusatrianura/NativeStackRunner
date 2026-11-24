package binary

import (
	"fmt"
	"runtime"
	"strings"
)

// ArchiveType indicates the compression type of the downloaded asset.
type ArchiveType string

const (
	ArchiveRaw   ArchiveType = "raw"
	ArchiveZip   ArchiveType = "zip"
	ArchiveTarGz ArchiveType = "tar.gz"
)

// Spec represents the metadata needed to download and locate a precompiled binary.
type Spec struct {
	Service       string
	Version       string
	OS            string
	Arch          string
	URL           string
	Type          ArchiveType
	BinarySubpath string // Path to executable inside archive or cache directory
	SHA256        string // Optional/Verified expected SHA-256 checksum
}

// ManifestResolver resolves download specifications for services.
type ManifestResolver interface {
	Resolve(service, version, targetOS, targetArch string) (*Spec, error)
}

// Manifest maps supported services to their download definitions.
type Manifest struct{}

// NewManifest creates a new Manifest resolver.
func NewManifest() *Manifest {
	return &Manifest{}
}

// Resolve looks up the download Spec for a service, requested version, target OS, and architecture.
func (m *Manifest) Resolve(service, version, targetOS, targetArch string) (*Spec, error) {
	service = strings.ToLower(strings.TrimSpace(service))
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}

	switch service {
	case "minio":
		return resolveMinIO(version, targetOS, targetArch)
	case "redis":
		return resolveRedis(version, targetOS, targetArch)
	case "postgres", "postgresql":
		return resolvePostgres(version, targetOS, targetArch)
	default:
		return nil, fmt.Errorf("unknown service %q: %w", service, ErrUnsupportedPlatform)
	}
}

func resolveMinIO(version, targetOS, targetArch string) (*Spec, error) {
	if version == "" || version == "latest" {
		version = "RELEASE.2024-01-18T22-51-28Z"
	}

	binName := "minio"
	if targetOS == "windows" {
		binName = "minio.exe"
	}

	var assetName, sha256Hash string
	switch fmt.Sprintf("%s-%s", targetOS, targetArch) {
	case "windows-amd64":
		assetName = "minio.windows-amd64.RELEASE.2024-01-18T22-51-28Z.exe"
		sha256Hash = "e6a467f9dcd4fb8282d795c9c93450afa243b666fbb8355d5bd48b280dd6dff6"
	case "linux-amd64":
		assetName = "minio.linux-amd64.RELEASE.2024-01-18T22-51-28Z"
		sha256Hash = "72b22c3cd66e03cf587328ed6c61fc19d95e1b1dc7ae7fcacb8c0801202c01de"
	case "linux-arm64":
		assetName = "minio.linux-arm64.RELEASE.2024-01-18T22-51-28Z"
		sha256Hash = "216663b9ebf38abe56d7515f71b14dbdf975b426c680adc30f9f0a24a48e68c5"
	case "darwin-amd64":
		assetName = "minio.darwin-amd64.RELEASE.2024-01-18T22-51-28Z"
		sha256Hash = "d8dd32f6737092a79014bc5a4dfdb7ee72e3b4a36920e615a226115e30c8bc4d"
	case "darwin-arm64":
		assetName = "minio.darwin-arm64.RELEASE.2024-01-18T22-51-28Z"
		sha256Hash = "1aa011118cb49b8205500dac2a5576c25296c0ba152ac8f5f7b8b303c233ebfd"
	default:
		return nil, fmt.Errorf("minio on %s/%s: %w", targetOS, targetArch, ErrUnsupportedPlatform)
	}

	url := fmt.Sprintf("https://github.com/minio/minio/releases/download/%s/%s", version, assetName)

	return &Spec{
		Service:       "minio",
		Version:       version,
		OS:            targetOS,
		Arch:          targetArch,
		URL:           url,
		Type:          ArchiveRaw,
		BinarySubpath: binName,
		SHA256:        sha256Hash,
	}, nil
}

func resolveRedis(version, targetOS, targetArch string) (*Spec, error) {
	switch targetOS {
	case "windows":
		if targetArch != "amd64" {
			return nil, fmt.Errorf("redis on %s/%s: %w", targetOS, targetArch, ErrUnsupportedPlatform)
		}
		return &Spec{
			Service:       "redis",
			Version:       "5.0.14",
			OS:            targetOS,
			Arch:          targetArch,
			URL:           "https://github.com/tporadowski/redis/releases/download/v5.0.14.1/Redis-x64-5.0.14.1.zip",
			Type:          ArchiveZip,
			BinarySubpath: "redis-server.exe",
			SHA256:        "018ea18a35876383cbb5f4cd0258adfc87747cf9d619bce1cf73a2e36f720ccf",
		}, nil

	case "linux", "darwin":
		archStr := "x86_64"
		hash := "5c0fe8dade33aa5d4496bf776ac2ef1c783be5dee80081020c511b3fa8d43298"
		if targetArch == "arm64" {
			archStr = "aarch64"
			hash = "c109be32aca6070b43ef7b2e5925f90307f6e54508307a04038b0305f762e14e"
		} else if targetArch != "amd64" {
			return nil, fmt.Errorf("redis on %s/%s: %w", targetOS, targetArch, ErrUnsupportedPlatform)
		}

		return &Spec{
			Service:       "redis",
			Version:       "dragonfly-1.14.0",
			OS:            targetOS,
			Arch:          targetArch,
			URL:           fmt.Sprintf("https://github.com/dragonflydb/dragonfly/releases/download/v1.14.0/dragonfly-%s.tar.gz", archStr),
			Type:          ArchiveTarGz,
			BinarySubpath: "dragonfly",
			SHA256:        hash,
		}, nil

	default:
		return nil, fmt.Errorf("redis on %s/%s: %w", targetOS, targetArch, ErrUnsupportedPlatform)
	}
}

func resolvePostgres(version, targetOS, targetArch string) (*Spec, error) {
	pgVersion := "16.1.0"
	if strings.HasPrefix(version, "15") {
		pgVersion = "15.5.0"
	} else if strings.HasPrefix(version, "14") {
		pgVersion = "14.10.0"
	}

	binSubpath := "bin/postgres"
	if targetOS == "windows" {
		binSubpath = "bin/postgres.exe"
	}

	postgresHashes := map[string]map[string]string{
		"windows-amd64": {
			"16.1.0":  "0eabe774cb4a26fee885a4e6b257a7a48ee19aaa81dbe6f31a07dfaaf7f852c3",
			"15.5.0":  "b0e40402ff77caf5ee5d030bb1f849968483df1c3010a4cb966495ad74d4dd14",
			"14.10.0": "5f9d2826ae847016dea455d9b47bc48115b021add62ac0f85c27976411272133",
		},
		"linux-amd64": {
			"16.1.0":  "0d8cd44e7dfc05fad0d6801140c3ecca11734b08beb9e2f216b8d5df52e40ed5",
			"15.5.0":  "f4ab44d1538c042876dc973992692ab4568c8fb5c5b5fbd13fa5780869a171f7",
			"14.10.0": "c61176d28e47cb362aad45fdb79582070a07ec9b8e388062768343d3f3b665d2",
		},
		"linux-arm64": {
			"16.1.0":  "2d56e292917ee7d977ad6535c59f80441c656a2b39ca612efe7bd68957371e81",
			"15.5.0":  "1e6e235ef090dd40de093a518c2b0374fa9983a317a9a77453eea2832e15402d",
			"14.10.0": "d49bd69a8b12ee49a6cdec53ff1ea1b06782a1a30e15aa359989cd790651d92b",
		},
		"darwin-amd64": {
			"16.1.0":  "90a106fd5a9224352375c0859570428bf31038c450057945ef27bbb08a46a971",
			"15.5.0":  "8ad36714c601d7937d882fa850a01d54e3c6df6fd21f6c1ba9c3066de210d6c7",
			"14.10.0": "ef8db6bea1a955336943f823e93f2c37a1f0bd9bf8b981937d30e561886a2262",
		},
		"darwin-arm64": {
			"16.1.0":  "8c03e720a4c6d113cc8515c88bf45a2703401be47e12df59e9d6005a3eb6ea70",
			"15.5.0":  "4065dca072403f6893ad281f6b8948fba8c695bb61fdad7108fd7049678b904a",
			"14.10.0": "58086f0e101ef57a8f77ebcc37e3d5ca3ffc387b59a0c80cb8fa86ef053ae025",
		},
	}

	osArch := fmt.Sprintf("%s-%s", targetOS, targetArch)
	versionMap, ok := postgresHashes[osArch]
	if !ok {
		return nil, fmt.Errorf("postgres on %s/%s: %w", targetOS, targetArch, ErrUnsupportedPlatform)
	}

	hash, ok := versionMap[pgVersion]
	if !ok {
		return nil, fmt.Errorf("postgres version %s on %s: %w", pgVersion, osArch, ErrUnsupportedPlatform)
	}

	artifactOS := targetOS
	artifactArch := targetArch
	if targetArch == "arm64" {
		artifactArch = "arm64v8"
	}
	artifactName := fmt.Sprintf("embedded-postgres-binaries-%s-%s", artifactOS, artifactArch)
	url := fmt.Sprintf("https://repo1.maven.org/maven2/io/zonky/test/postgres/%s/%s/%s-%s.jar",
		artifactName, pgVersion, artifactName, pgVersion)

	return &Spec{
		Service:       "postgres",
		Version:       pgVersion,
		OS:            targetOS,
		Arch:          targetArch,
		URL:           url,
		Type:          ArchiveZip,
		BinarySubpath: binSubpath,
		SHA256:        hash,
	}, nil
}

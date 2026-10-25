+++
title = "Verifying a release"
description = "Verify chart2kro release archives and container images with cosign and GitHub build-provenance attestations."
weight = 90

[extra]
group = "Reference"
nav = "Verifying releases"
+++

Every release is signed with Sigstore (keyless, by the release workflow) and has a GitHub build-provenance attestation.

**Archives.** The checksum file is signed; it covers every archive.

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/hupe1980/chart2kro/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --check --ignore-missing checksums.txt
```

**Provenance.**

```bash
gh attestation verify chart2kro_*_linux_amd64.tar.gz --repo hupe1980/chart2kro
```

**Image signature.**

```bash
cosign verify ghcr.io/hupe1980/chart2kro:<version> \
  --certificate-identity-regexp '^https://github.com/hupe1980/chart2kro/\.github/workflows/release\.yaml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

**Image provenance.**

```bash
gh attestation verify oci://ghcr.io/hupe1980/chart2kro:<version> --repo hupe1980/chart2kro
```

Each archive also has an SPDX SBOM.

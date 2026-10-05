# Changelog

## [0.1.4] - 2026-10-05


### Changed

- Release train train-2026.10.3 (core v0.6.15): default muxcored image `v0.6.15` in the API default, kubebuilder marker, CRD, samples, README and soak script.

## [0.1.3] - 2026-10-05


### Changed
- Release train train-2026.10.2 (core v0.6.13): default muxcored image `v0.6.13` in the API default, kubebuilder marker, CRD, samples, README and soak script.

## [0.1.2] - 2026-10-05


### Changed
- Default muxcored image pinned to v0.6.7 (release train train-2026.10.1, FR-INS-006): API default, CRD, samples, README, soak script.

## [0.1.1] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

### Added
- `scripts/publish-operator-local.sh` — LAN/GHCR OCI publish (insecure localhost registry auto-config)
- `scripts/soak-operator-kind.sh` — controller tests + optional kind reconcile smoke
- CI `image` job builds operator container (`BUILD_ONLY=1`)

## [v0.1.0] — 2026-08-10

### Added
- `MuxCorePlatform` CRD (`muxcore.media/v1alpha1`) for minimal platform (muxcored + sidecars)
- controller-runtime reconciler creating Deployments + Services with owner references
- Sample CR mirroring `mvp/deploy` minimal platform module set
- Kind/k3d local-image samples under `config/samples/kind/` (`localhost:5000` / cluster-loaded tags)
- README day-1 preference: installer/compose over operator
- Unit test with fake client

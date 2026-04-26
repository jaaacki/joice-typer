verify: bridge-contract-check
	$(MAKE) test
	$(MAKE) frontend-build
	$(MAKE) vet-cross
	$(MAKE) verify-buildinfra

verify-buildinfra:
	go test -count=1 ./internal/buildinfra

verify-mac: verify app-no-version-bump

verify-windows: bridge-contract-check vet-cross verify-buildinfra
	@echo "Windows packaging preflight and full runtime packaging require Windows toolchain artifacts."
	@echo "Run build-windows-runtime-amd64-no-version-bump and package-windows-no-version-bump on a Windows-capable builder."

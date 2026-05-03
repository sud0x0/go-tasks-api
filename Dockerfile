# This image is consumed by GoReleaser. The `api` binary is cross-compiled
# on the runner before this build context is assembled, so the image just
# copies it in — no Go toolchain inside the final image.
#
# To produce a snapshot image locally without publishing:
#     make prod-build
#
# To produce the published image, push a tag (`git push --tags`) and
# .github/workflows/release.yml will run GoReleaser end-to-end.
FROM gcr.io/distroless/static-debian12

# Run as the nonroot user that distroless ships with (uid 65532).
USER nonroot:nonroot

COPY go-tasks-api /go-tasks-api

EXPOSE 8080

ENTRYPOINT ["/go-tasks-api"]

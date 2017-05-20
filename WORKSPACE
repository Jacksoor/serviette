git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "3b3c1e6d6d12f00fb462e945805c1c8c5d9ea1dc",
)

git_repository(
    name = "com_github_grpc_grpc",
    remote = "https://github.com/grpc/grpc.git",
    init_submodules = True,
    commit = "3808b6efe66b87269d43847bc113e94e2d3d28fb",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories", "new_go_repository")

go_repositories()

new_go_repository(
  name = "com_github_golang_protobuf",
  importpath = "github.com/golang/protobuf",
  commit = "8ee79997227bf9b34611aee7946ae64735e6fd93",
)

http_archive(
    name = "com_github_google_protobuf",
    url = "https://github.com/google/protobuf/archive/v3.2.0.tar.gz",
    strip_prefix = "protobuf-3.2.0",
    sha256 = "2a25c2b71c707c5552ec9afdfb22532a93a339e1ca5d38f163fe4107af08c54c",
)

new_go_repository(
    name = "org_golang_google_grpc",
    tag = "v1.3.0",
    importpath = "google.golang.org/grpc",
)

new_go_repository(
    name = "org_golang_google_genproto",
    commit = "bb3573be0c484136831138976d444b8754777aff",
    importpath = "google.golang.org/genproto",
)

new_go_repository(
    name = "com_github_golang_glog",
    commit = "23def4e6c14b4da8ac2ed8007337bc5eb5007998",
    importpath = "github.com/golang/glog",
)

new_go_repository(
    name = "org_golang_x_crypto",
    commit = "efac7f277b17c19894091e358c6130cb6bd51117",
    importpath = "golang.org/x/crypto",
)

new_go_repository(
    name = "org_golang_x_net",
    commit = "513929065c19401a1c7b76ecd942f9f86a0c061b",
    importpath = "golang.org/x/net",
)

new_go_repository(
    name = "org_golang_x_text",
    commit = "19e51611da83d6be54ddafce4a4af510cb3e9ea4",
    importpath = "golang.org/x/text",
)

new_go_repository(
    name = "com_github_mattn_go_sqlite3",
    commit = "cf7286f069c3ef596efcc87781a4653a2e7607bd",
    importpath = "github.com/mattn/go-sqlite3",
    build_tags = ["darwin", "linux"],
)

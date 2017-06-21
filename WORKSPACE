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
    name = "org_golang_x_sync",
    commit = "f52d1811a62927559de87708c8913c1650ce4f26",
    importpath = "golang.org/x/sync",
)

new_go_repository(
    name = "org_golang_x_oauth2",
    commit = "f047394b6d14284165300fd82dad67edb3a4d7f6",
    importpath = "golang.org/x/oauth2",
)

new_go_repository(
    name = "com_github_lib_pq",
    commit = "8837942c3e09574accbc5f150e2c5e057189cace",
    importpath = "github.com/lib/pq",
)

new_go_repository(
    name = "com_github_bwmarrin_discordgo",
    commit = "eadd2d027c2530d056be7f51bbc69a260ba5cfdf",
    importpath = "github.com/bwmarrin/discordgo",
    remote = "https://github.com/porpoises/discordgo.git",
    vcs = "git",
)

new_go_repository(
    name = "com_github_gorilla_websocket",
    commit = "a91eba7f97777409bc2c443f5534d41dd20c5720",
    importpath = "github.com/gorilla/websocket",
)

new_go_repository(
    name = "com_github_hako_durafmt",
    commit = "83a6d8dc879e5db09185e352561da4326f443de6",
    importpath = "github.com/hako/durafmt",
)

new_go_repository(
    name = "com_github_julienschmidt_httprouter",
    commit = "975b5c4c7c21c0e3d2764200bf2aa8e34657ae6e",
    importpath = "github.com/julienschmidt/httprouter",
)

new_go_repository(
    name = "com_github_djherbis_buffer",
    commit = "81a3204d823f2cb127fd516387fab63abe1017f3",
    importpath = "github.com/djherbis/buffer",
)

new_go_repository(
    name = "com_github_justinas_nosurf",
    commit = "8e15682772641a1e39c431233e6a9338a32def32",
    importpath = "github.com/justinas/nosurf",
)

new_go_repository(
    name = "com_github_emicklei_go_restful",
    commit = "ff4f55a206334ef123e4f79bbf348980da81ca46",
    importpath = "github.com/emicklei/go-restful",
)

new_go_repository(
    name = "com_github_dgrijalva_jwt_go",
    commit = "a539ee1a749a2b895533f979515ac7e6e0f5b650",
    importpath = "github.com/dgrijalva/jwt-go",
)

new_go_repository(
    name = "com_github_prometheus_client_golang",
    commit = "de4d4ffe63b9eff7f27484fdef6e421597e6abb4",
    importpath = "github.com/prometheus/client_golang",
)

new_go_repository(
    name = "com_github_prometheus_client_model",
    commit = "6f3806018612930941127f2a7c6c453ba2c527d2",
    importpath = "github.com/prometheus/client_model",
)

new_go_repository(
    name = "com_github_prometheus_common",
    commit = "0d0c3d572886e0f2323ed376557f9eb99b97d25b",
    importpath = "github.com/prometheus/common",
)

new_go_repository(
    name = "com_github_prometheus_procfs",
    commit = "a3bfc74126ea9e45ee5d5c6f7fc86191b7d488fb",
    importpath = "github.com/prometheus/procfs",
)

new_go_repository(
    name = "com_github_beorn7_perks",
    commit = "4c0e84591b9aa9e6dcfdf3e020114cd81f89d5f9",
    importpath = "github.com/beorn7/perks",
)

new_go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    commit = "c12348ce28de40eed0136aa2b644d0ee0650e56c",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
)

new_go_repository(
    name = "org_golang_google_appengine",
    commit = "a2f4131514e563cedfdb6e7d267df9ad48591e93",
    importpath = "google.golang.org/appengine",
)

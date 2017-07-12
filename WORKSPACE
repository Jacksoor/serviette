git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "936af5753ebcd7a1f05127678435389cc2e3db5d",
)

git_repository(
    name = "com_github_grpc_grpc",
    remote = "https://github.com/grpc/grpc.git",
    init_submodules = True,
    commit = "3808b6efe66b87269d43847bc113e94e2d3d28fb",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories", "go_repository")

go_repositories()

go_repository(
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

go_repository(
    name = "org_golang_google_grpc",
    tag = "v1.3.0",
    importpath = "google.golang.org/grpc",
)

go_repository(
    name = "org_golang_google_genproto",
    commit = "bb3573be0c484136831138976d444b8754777aff",
    importpath = "google.golang.org/genproto",
)

go_repository(
    name = "com_github_golang_glog",
    commit = "23def4e6c14b4da8ac2ed8007337bc5eb5007998",
    importpath = "github.com/golang/glog",
)

go_repository(
    name = "org_golang_x_crypto",
    commit = "efac7f277b17c19894091e358c6130cb6bd51117",
    importpath = "golang.org/x/crypto",
)

go_repository(
    name = "org_golang_x_net",
    commit = "513929065c19401a1c7b76ecd942f9f86a0c061b",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "org_golang_x_text",
    commit = "19e51611da83d6be54ddafce4a4af510cb3e9ea4",
    importpath = "golang.org/x/text",
)

go_repository(
    name = "org_golang_x_sync",
    commit = "f52d1811a62927559de87708c8913c1650ce4f26",
    importpath = "golang.org/x/sync",
)

go_repository(
    name = "org_golang_x_oauth2",
    commit = "f047394b6d14284165300fd82dad67edb3a4d7f6",
    importpath = "golang.org/x/oauth2",
)

go_repository(
    name = "org_golang_x_sys",
    commit = "abf9c25f54453410d0c6668e519582a9e1115027",
    importpath = "golang.org/x/sys",
    build_tags = ["linux", "amd64", "darwin"],
)

go_repository(
    name = "com_github_lib_pq",
    commit = "8837942c3e09574accbc5f150e2c5e057189cace",
    importpath = "github.com/lib/pq",
)

go_repository(
    name = "com_github_bwmarrin_discordgo",
    commit = "eadd2d027c2530d056be7f51bbc69a260ba5cfdf",
    importpath = "github.com/bwmarrin/discordgo",
    remote = "https://github.com/porpoises/discordgo.git",
    vcs = "git",
)

go_repository(
    name = "com_github_gorilla_websocket",
    commit = "a91eba7f97777409bc2c443f5534d41dd20c5720",
    importpath = "github.com/gorilla/websocket",
)

go_repository(
    name = "com_github_hako_durafmt",
    commit = "83a6d8dc879e5db09185e352561da4326f443de6",
    importpath = "github.com/hako/durafmt",
)

go_repository(
    name = "com_github_julienschmidt_httprouter",
    commit = "975b5c4c7c21c0e3d2764200bf2aa8e34657ae6e",
    importpath = "github.com/julienschmidt/httprouter",
)

go_repository(
    name = "com_github_djherbis_buffer",
    commit = "81a3204d823f2cb127fd516387fab63abe1017f3",
    importpath = "github.com/djherbis/buffer",
)

go_repository(
    name = "com_github_justinas_nosurf",
    commit = "8e15682772641a1e39c431233e6a9338a32def32",
    importpath = "github.com/justinas/nosurf",
)

go_repository(
    name = "com_github_emicklei_go_restful",
    commit = "ff4f55a206334ef123e4f79bbf348980da81ca46",
    importpath = "github.com/emicklei/go-restful",
)

go_repository(
    name = "com_github_dgrijalva_jwt_go",
    commit = "a539ee1a749a2b895533f979515ac7e6e0f5b650",
    importpath = "github.com/dgrijalva/jwt-go",
)

go_repository(
    name = "com_github_prometheus_client_golang",
    commit = "de4d4ffe63b9eff7f27484fdef6e421597e6abb4",
    importpath = "github.com/prometheus/client_golang",
)

go_repository(
    name = "com_github_prometheus_client_model",
    commit = "6f3806018612930941127f2a7c6c453ba2c527d2",
    importpath = "github.com/prometheus/client_model",
)

go_repository(
    name = "com_github_prometheus_common",
    commit = "0d0c3d572886e0f2323ed376557f9eb99b97d25b",
    importpath = "github.com/prometheus/common",
)

go_repository(
    name = "com_github_prometheus_procfs",
    commit = "a3bfc74126ea9e45ee5d5c6f7fc86191b7d488fb",
    importpath = "github.com/prometheus/procfs",
)

go_repository(
    name = "com_github_beorn7_perks",
    commit = "4c0e84591b9aa9e6dcfdf3e020114cd81f89d5f9",
    importpath = "github.com/beorn7/perks",
)

go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    commit = "c12348ce28de40eed0136aa2b644d0ee0650e56c",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
)

go_repository(
    name = "org_golang_google_appengine",
    commit = "a2f4131514e563cedfdb6e7d267df9ad48591e93",
    importpath = "google.golang.org/appengine",
)

go_repository(
    name = "com_github_opencontainers_runc",
    commit = "a0159fddcd197a57790b6dd5654802b7dc8f80be",
    importpath = "github.com/opencontainers/runc",
    build_tags = ["linux", "cgo", "seccomp"],
)

go_repository(
    name = "com_github_opencontainers_selinux",
    commit = "4a2974bf1ee960774ffd517717f1f45325af0206",
    importpath = "github.com/opencontainers/selinux",
)

go_repository(
    name = "com_github_opencontainers_runtime_spec",
    commit = "198f23f827eea397d4331d7eb048d9d4c7ff7bee",
    importpath = "github.com/opencontainers/runtime-spec",
)

go_repository(
    name = "com_github_vishvananda_netlink",
    commit = "4e28683688429fdf8413cc610d59fb1841986300",
    importpath = "github.com/vishvananda/netlink",
)

go_repository(
    name = "com_github_vishvananda_netns",
    commit = "54f0e4339ce73702a0607f49922aaa1e749b418d",
    importpath = "github.com/vishvananda/netns",
)

go_repository(
    name = "com_github_syndtr_gocapability",
    commit = "e7cb7fa329f456b3855136a2642b197bad7366ba",
    importpath = "github.com/syndtr/gocapability",
)

go_repository(
    name = "com_github_mrunalp_fileutils",
    commit = "4ee1cc9a80582a0c75febdd5cfa779ee4361cbca",
    importpath = "github.com/mrunalp/fileutils",
)

go_repository(
    name = "com_github_docker_docker",
    commit = "4964b092384a0e1c42b2db728aeeaf129a50f54f",
    importpath = "github.com/docker/docker",
)

go_repository(
    name = "com_github_docker_go_units",
    commit = "0dadbb0345b35ec7ef35e228dabb8de89a65bf52",
    importpath = "github.com/docker/go-units",
)

go_repository(
    name = "com_github_coreos_go_systemd",
    commit = "24036eb3df68550d24a2736c5d013f4e83366866",
    importpath = "github.com/coreos/go-systemd",
)

go_repository(
    name = "com_github_coreos_pkg",
    commit = "8dbaa491b063ed47e2474b5363de0c0db91cf9f2",
    importpath = "github.com/coreos/pkg",
)

go_repository(
    name = "com_github_Sirupsen_logrus",
    commit = "3d4380f53a34dcdc95f0c1db702615992b38d9a4",
    importpath = "github.com/Sirupsen/logrus",
)

go_repository(
    name = "com_github_godbus_dbus",
    commit = "37252881b3a87eaa2eb04b0ff2211f54f45199ab",
    importpath = "github.com/godbus/dbus",
)

go_repository(
    name = "com_github_pkg_errors",
    commit = "c605e284fe17294bda444b34710735b29d1a9d90",
    importpath = "github.com/pkg/errors",
)

go_repository(
    name = "com_github_kballard_go_shellquote",
    commit = "cd60e84ee657ff3dc51de0b4f55dd299a3e136f2",
    importpath = "github.com/kballard/go-shellquote",
)

new_git_repository(
    name = "com_github_seccomp_libseccomp_golang",
    remote = "https://github.com/seccomp/libseccomp-golang",
    commit = "e3496e3a417d1dc9ecdceca5af2513271fed37a0",
    build_file_content = """
package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "cgo_genrule",
    "go_library",
    "go_prefix",
)

go_prefix("github.com/seccomp/libseccomp-golang")

cgo_genrule(
    name = "cgo_codegen",
    srcs = [
        "seccomp.go",
        "seccomp_internal.go",
    ],
    clinkopts = [
        "-lz",
        "-lm",
        "-lpthread",
        "-ldl",
        "-lseccomp",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [],
    library = ":cgo_codegen",
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
)
"""
)

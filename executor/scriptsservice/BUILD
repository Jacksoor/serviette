load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/accounts:go_default_library",
        "//executor/scripts:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "@com_github_djherbis_buffer//limio:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_golang_protobuf//proto:go_default_library",
        "@com_github_opencontainers_runc//libcontainer/cgroups:go_default_library",
        "@com_github_prometheus_client_golang//prometheus:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)

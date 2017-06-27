load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//delegator/supervisor/rpc:go_default_library",
        "//delegator/supervisor/rpc/messagingservice:go_default_library",
        "//delegator/supervisor/rpc/networkinfoservice:go_default_library",
        "//delegator/supervisor/rpc/outputservice:go_default_library",
        "//delegator/supervisor/rpc/statsservice:go_default_library",
        "//delegator/supervisor/rpc/supervisorservice:go_default_library",
        "//executor/accountsservice/v1pb:go_default_library",
        "//executor/messagingservice/v1pb:go_default_library",
        "//executor/networkinfoservice/v1pb:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/statsservice/v1pb:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_golang_protobuf//jsonpb:go_default_library",
        "@com_github_golang_protobuf//proto:go_default_library",
        "@com_github_opencontainers_runc//libcontainer:go_default_library",
        "@com_github_opencontainers_runc//libcontainer/cgroups:go_default_library",
        "@com_github_opencontainers_runc//libcontainer/configs:go_default_library",
        "@com_github_opencontainers_runc//libcontainer/nsenter:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_x_net//context:go_default_library",
        "@org_golang_x_sys//unix:go_default_library",
    ],
)

go_binary(
    name = "supervisor",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

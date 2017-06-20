load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["service.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/accounts:go_default_library",
        "//executor/messagingservice/v1pb:go_default_library",
        "//executor/networkinfoservice/v1pb:go_default_library",
        "//executor/scripts:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/statsservice/v1pb:go_default_library",
        "//executor/worker:go_default_library",
        "//executor/worker/rpc/messagingservice:go_default_library",
        "//executor/worker/rpc/networkinfoservice:go_default_library",
        "//executor/worker/rpc/outputservice:go_default_library",
        "//executor/worker/rpc/statsservice:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_golang_protobuf//jsonpb:go_default_library",
        "@com_github_prometheus_client_golang//prometheus:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//codes:go_default_library",
        "@org_golang_x_net//context:go_default_library",
    ],
)

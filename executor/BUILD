load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    data = [
        "//clients",
    ],
    visibility = ["//visibility:private"],
    deps = [
        "//executor/accounts:go_default_library",
        "//executor/accountsservice:go_default_library",
        "//executor/accountsservice/v1pb:go_default_library",
        "//executor/scripts:go_default_library",
        "//executor/scriptsservice:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//executor/webdav:go_default_library",
        "//executor/worker:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@com_github_mattn_go_sqlite3//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_google_grpc//grpclog/glogger:go_default_library",
        "@org_golang_google_grpc//reflection:go_default_library",
        "@org_golang_x_net//trace:go_default_library",
    ],
)

go_binary(
    name = "executor",
    gc_linkopts = [
        "-linkmode",
        "external",
        "-extldflags",
        "-static",
    ],
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

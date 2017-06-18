load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//executor/accountsservice/v1pb:go_default_library",
        "//executor/scriptsservice/v1pb:go_default_library",
        "//restbridge/rest:go_default_library",
        "@com_github_emicklei_go_restful//:go_default_library",
        "@com_github_golang_glog//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
        "@org_golang_x_net//trace:go_default_library",
    ],
)

go_binary(
    name = "webbridge",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "restbridge",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
)

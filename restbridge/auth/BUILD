load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["authenticator.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//executor/accountsservice/v1pb:go_default_library",
        "@com_github_dgrijalva_jwt_go//:go_default_library",
        "@com_github_emicklei_go_restful//:go_default_library",
    ],
)

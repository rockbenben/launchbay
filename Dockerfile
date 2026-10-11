# 构建期：Go 静态编译。web/ 已用 go:embed 打进二进制，不需要 node 阶段。
FROM golang:1.27-alpine AS build
# buildx 注入的内置 ARG：声明之后才能在下面的 RUN 里用。多平台构建时它分别是
# amd64 / arm64，交叉编译与"跳过 arm64 的测试"这两件事都靠它。
ARG TARGETARCH
WORKDIR /src
# tzdata 装在构建期，运行期只拷需要的目录：scratch 里没有系统时区库，
# 不装的话 created_at 的时区偏移会是假的（见下面 stage-1 的注释）。
RUN apk add --no-cache tzdata
# 分层缓存：先依赖后源码。改一行代码不该重下模块。
COPY go.mod ./
RUN go mod download
COPY . .
# 门禁在镜像构建里跑一遍：CI 绿不代表这个镜像可用。
# 跑的是**应用代码**的门禁——tools/ 被 .dockerignore 排除（leakguard 的判据是
# git ls-files，而这里没有 .git；genicons 要连 registry），所以 `./...` 匹配到的
# 包比仓库里少一半。仓库级的闸由 .github/workflows/ci.yml 跑，两处别互相冒充。
#
# **门禁只在 amd64 上跑**：发布 workflow 用 buildx 同时构建 amd64 与 arm64，arm64
# 那一路跑在 QEMU 模拟下，同一份测试会慢一个数量级、几分钟后超时。而门禁跑一次
# 就够了——两个架构跑**同一批断言**不会多抓出任何东西，它们读的是同一份源码。
# 该被架构抓出来的问题由交叉编译本身抓（链接错、平台专属代码编译不过），
# 那不需要跑测试。
RUN if [ "$TARGETARCH" = "amd64" ]; then go vet ./... && go test ./...; fi \
    && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/launchbay ./cmd/launchbay

FROM scratch
COPY --from=build /out/launchbay /launchbay
# 没有 shell：healthcheck 只能靠二进制自己的子命令（Task 12 Step 2）。
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# compose 设了 TZ=Asia/Shanghai，但 scratch 里没有 tzdata：Go 的 time.Local
# 找不到时区库会退回 UTC，created_at 的偏移就成了假话。把区域表拷进来，
# TZ 才真的生效。
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
USER 65534:65534
EXPOSE 7600
ENTRYPOINT ["/launchbay"]

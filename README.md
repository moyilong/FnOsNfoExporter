# 飞牛 NFO 导出器

原生 Go 服务 + fnOS FPK 封装。直接读取 NAS 上的飞牛影视数据库，使用数据库中原始绝对路径，在媒体旁生成 NFO。无需 Windows 盘符映射，不注入或修改 trim.media。

## 已实现

- 网页控制台：保存数据库/媒体目录设置、预览、立即导出、运行结果。
- 每日计划：选择 NAS 本地时间，默认关闭；应用运行时生效，不补跑关机期间的任务。
- 手动与计划任务互斥，导出最长运行 30 分钟。
- 数据库以 SQLite mode=ro / query_only 打开，在只读事务中取得一致的元数据视图；不复制运行中的数据库，不使用 immutable 绕过 WAL。
- Movie / Episode 按真实 type 判断，不把电影默认的 0 季/0 集误判为剧集。
- 电影和单集写入视频同名 NFO，避免合集目录中的 movie.nfo 冲突。
- 从 Episode → Season → TV 关系获取剧集信息，使用单集共同目录推导 tvshow.nfo；无法安全确定目录时跳过并报告。非标准目录请先查看预览。
- 标题、原名、简介、日期、评分、票数、外部 ID、季集编号、演员、导演和 Genre。
- 默认仅补缺失：已有 NFO 不覆盖，内容一致不改变时间戳。临时文件完整写入后发布；目标同时被其他程序创建时保留对方文件。
- 媒体必须实际存在；允许导出的媒体目录留空时导出全部，填写时仅导出配置目录内的媒体。解析符号链接检查边界，跳过目标 NFO 符号链接。
- 控制台管理员登录，配置及上次报告保存在 TRIM_PKGVAR，重启保留。
- GitHub CI：测试、amd64/arm64 交叉编译、官方 fnpack 打包、tag 自动发布 Release。

## 安装和使用

1. 将此项目推送至你的 GitHub 仓库，运行 Actions；普通 main/master 推送也会构建安装包。
2. 在 Actions artifacts 中下载对应架构 FPK：amd64 用于 x86_64，arm64 用于 ARM64。
3. 飞牛应用中心手动安装，向导中设置至少 12 字符的控制台密码。
4. 从应用图标打开控制台（新标签页），或访问 http://NAS_IP:18080。用户名 admin。
5. 默认优先使用 `/usr/local/apps/@appdata/trim.media/database/trimmedia.db`；确认数据库文件路径。允许导出的媒体目录留空时导出数据库中所有媒体的 NFO；需要限制范围时填写实际媒体目录，一行一个，例如：

   /vol02/1000-0-417451a0/电影
   /vol02/1000-0-417451a0/电视剧

6. 保存设置，先点“预览待导出内容”，确认后点“立即导出”。
7. 需要自动化时，开启每天自动导出并保存。

只访问一个真实的 trimmedia.db；同目录 WAL/SHM 必须可用，不要只把活动数据库主文件复制到另一个位置。数据库访问权限与媒体路径权限不足会在任务中报错。自动发现可能选到旧库，请确认路径。

应用需要访问飞牛影视的私有数据库，当前 FPK 以 root 运行；管理员可选配置导出根目录以限制范围，留空不限制媒体目录，数据库只读。控制台是独立认证，不自动继承飞牛登录态；默认 HTTP 服务适合可信局域网，远程使用应放在 HTTPS 反向代理后。

### 控制台账号与密码重设

用户名固定为 `admin`，密码为安装向导中自行设置的密码，没有默认密码，也不是飞牛系统的登录密码。程序只保存 `password.sha256`，不能从中找回原密码。

忘记密码时，先在应用中心停止应用，通过 NAS SSH 使用实际安装的程序路径和应用数据目录执行以下命令，然后在应用中心重新启动。`--data` 必须指向原来的 `TRIM_PKGVAR` 目录（其中应有 `password.sha256`），不能使用新的空目录。

```sh
read -r -s -p '新控制台密码（至少 12 字符）: ' wizard_password
echo
export wizard_password
/实际安装路径/fnos-nfo-exporter --data /实际应用数据目录 --init
unset wizard_password
```

以上命令在 Bash 中运行，需要有应用数据目录的写权限。重设只更新密码文件，保留导出配置和报告；必须重启服务才会加载新密码。

## 与飞牛扫描的关系

应用不会因数据库更新事件立即重写 NFO。手动或每日任务只补缺失文件；已存在的不同内容也跳过，所以不会在“数据库 → NFO → 扫描”之间反复更新时间戳。首批新增 NFO 仍可能触发一次扫描。可在飞牛媒体库启用“优先读取本地 NFO 和图片”。

已有错误匹配会忠实导出，工具不重新刮削。之前错误的 NFO 不会自动修正；应先人工修正或移走指定 NFO，再执行。

## 首版范围和限制

- 导出文字元数据；不复制私有海报缓存，不写飞牛内部缓存 URL，避免生成失效图片引用。
- 导出 TV 与单集，不单独输出 season.nfo；ISO 当普通媒体文件处理，目录形式 BDMV 跳过。
- tvshow.nfo 的目录依赖共同路径推断；单季命名只识别 Season 空格或下划线前缀，其他布局请人工检查。
- 为避免覆写，使用同目录临时文件 + 硬链接发布；不支持硬链接的挂载会报告错误而非退回不安全覆盖。
- 控制台仅显示前 500 条结果，完整上次报告在 TRIM_PKGVAR/last-run.json。
- 只保留上次运行报告、当前和上次启动日志。
- 当前已在提供的数据库中核对表结构、类型和关联，但未连接真实 NAS 完成安装测试。
- 本地网络限制阻止 SQLite Go 模块与 fnpack 下载，因此完整应用/FPK 尚未本机编译验证。核心标准库测试可独立运行；完整测试由 CI 执行。go.sum 将由首次联网 go mod tidy 生成，请首次成功构建后提交它。

## 开发和发布

Go 1.23+，纯 Go SQLite 驱动，无 CGO。

    go mod tidy
    go test -race ./...
    go vet ./...
    go build ./cmd/fnos-nfo-exporter

Windows 本地构建与启动（在项目根目录的 PowerShell 中执行）：

```powershell
go mod tidy
go build -o ./bin/fnos-nfo-exporter.exe ./cmd/fnos-nfo-exporter
if ($LASTEXITCODE -ne 0) { throw '构建失败' }
$env:wizard_password = 'LocalTest123456!'
./bin/fnos-nfo-exporter.exe --data ./data --init
if ($LASTEXITCODE -ne 0) { throw '密码初始化失败' }
Remove-Item Env:wizard_password
./bin/fnos-nfo-exporter.exe --data ./data --listen 127.0.0.1:18080
```

打开 http://127.0.0.1:18080，用户名 `admin`，密码 `LocalTest123456!`（仅为本地测试示例，可自行修改）。按 Ctrl+C 停止。后续启动只执行最后一行；再次执行 `--init` 会重设该数据目录的密码。Windows 可测试控制台；实际导出需要配置本机可访问的数据库和媒体路径，NAS 的 `/vol...` 路径应在 NAS 上测试。

Linux / Bash 本地运行：

```sh
wizard_password='LocalTest123456!' go run ./cmd/fnos-nfo-exporter --data ./data --init
go run ./cmd/fnos-nfo-exporter --data ./data --listen 127.0.0.1:18080
```

只读检查数据库：

    go run ./cmd/fnos-nfo-exporter --inspect /absolute/path/trimmedia.db

FPK 构建（Linux，先安装官方 fnpack 1.2.3）：

    bash scripts/build-fpk.sh amd64 0.1.0
    bash scripts/build-fpk.sh arm64 0.1.0

推送版本标签 v0.1.0 后，GitHub Actions 在测试成功后构建两种架构，上传 FPK 和 SHA-256 校验文件，自动发布 Release。建议先用普通分支构建的安装包完成 NAS 安装验证，再推送正式版本标签。CI 使用 packaging 白名单构建，不打包数据库、配置口令或 Windows 项目产物。

## 布局

- cmd/fnos-nfo-exporter：Go 服务、网页、SQLite 集成测试
- internal/core：元数据导出与文件写入，独立单元测试
- packaging：官方 FPK 目录结构、manifest、生命周期、向导、桌面入口
- scripts/build-fpk.sh：构建打包
- .github/workflows/release.yml：CI 和发布
- tools/inspect.ps1：Windows SQLite 只读检查工具
- tools/icon：标准库生成应用图标

参考：[飞牛 fnpack](https://developer.fnnas.com/docs/cli/fnpack/)、[应用框架](https://developer.fnnas.com/docs/core-concepts/framework/)、[Manifest](https://developer.fnnas.com/docs/core-concepts/manifest/)。

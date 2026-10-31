# 离线缓存与双向同步

本文说明 NewPipe fork（App）与 `videod`（后端）之间"待看列表 ↔ 本地文件"的关系：
B 站/YouTube 上删了会怎样、本地标记看过会怎样、能不能回写、以及已知缺口。

## App 发起同步的流程

App 通过 `VideodClient` 访问同一设备上的 `http://127.0.0.1:8732`，由 `videod` 持有平台 Cookie、执行远端读取并写数据库；NewPipe 不直接请求 YouTube 或 Bilibili 的同步接口。连接失败、超时或 HTTP 非成功状态会作为客户端异常返回。

| App 操作 | 同步请求 | 后续动作 |
|---|---|---|
| 打开离线库 | 不自动同步；调用 `GET /api/library` 和历史记录读取 | 展示服务端已有下载及播放进度 |
| 离线库下拉刷新 | 对当前选中的来源调用 `POST /api/sync`，范围 `watch_later`；“全部”发送 YouTube 后 Bilibili 两次请求 | 再读全部离线库与播放进度并刷新列表；逐来源错误以 Toast 显示 |
| 切换来源标签 | 不同步 | 只筛选当前已读取的离线库 |
| 进入“已看”标签 | 对所有来源请求 `watch_later` 同步 | 再读取并筛选已标记项目；这个标签将远端待看列表中消失的下载称作“已看” |
| 打开“源”页面 | 不同步；分别读取 `GET /api/watch_later?account_id=…` 和 `GET /api/library?account_id=…` | 展示已同步的待看条目及其是否已下载；点击未下载条目会 `POST /api/downloads` 并重读该来源列表，没有下载进度展示；刷新远端数据需用离线库刷新或设置页立即同步 |
| 设置页“立即同步” | 先读 `GET /api/settings`；根据 `source_youtube`、`source_bilibili` 开关依次同步启用来源，范围固定为 `watch_later`，并传入 `auto_download` 设置 | 读取存储统计及来源分解，逐来源展示结果和耗时 |

离线库刷新使用单个运行标记避免并发刷新；网络请求在 IO 线程执行，完成后切回主线程更新界面。所有同步都是前台操作；从当前代码路径看不到周期性任务或 App 启动时的后台同步。`source_sync` 和 `sync_interval_min` 虽有服务端设置默认值，但这不代表定时调度已接入：本文所查的 App 调用路径没有使用它们安排任务。

单来源请求由客户端传入来源名；未给来源时 `VideodClient.sync(SyncParams)` 会按 YouTube、Bilibili 顺序逐个请求并聚合错误。Bilibili 对 `all`、`subscriptions`、`playlists` 范围会降为 `watch_later`；但当前离线库和设置页入口本身只请求 `watch_later`。服务端同步处理的 API 合约及字段见 [API.md](API.md#catalog)。

客户端使用 30 秒连接超时和 30 秒读取超时。服务端 provider 同步的整体 context 上限为 5 分钟，provider 命令另有单次超时；因此客户端读取超时可能先于服务端任务的总时限到达。`/api/sync` 将 provider 级别失败放在成功 HTTP 响应中的 `errors` 字段，客户端会解析并显示；请求级 HTTP 错误则作为异常处理。

## 两套数据，彼此不联动

| 表 | 内容 | 谁写 |
|---|---|---|
| `watch_later` / `playlist_videos(WL)` | 待看列表成员（只有 id 和顺序） | `POST /api/sync` 每次整体替换 |
| `remote_missing` | 已下载视频不再出现在来源待看列表的标记与时间 | 成功 sync 对比前后列表 |
| `downloads` | 下载任务与文件路径 | 下载队列 / 本地删除 |
| `videos` | 元数据（标题、封面、时长） | sync 时 upsert |

`GET /api/library` 读 `downloads`（`state='completed' AND output_path<>''`），并附带
`remote_missing_since`；`GET /api/watch_later` 读 `watch_later`。

## 远端缺失与本地文件

成功同步一次 B 站/YouTube 待看列表后，服务比较同步前已有的列表成员与远端当前列表：

- 之前在待看列表、当前不在、而且已有完成下载的项目，会写入 `remote_missing`。
- 离线库副标题会显示"No longer in YouTube / Bilibili watch later"；存储分析把这些文件
  汇总为"已从来源待看列表移除"，显示文件数、空间大小和条目。
- 只标记，不删 `videos`、下载记录或文件。用户可在离线库长按单条删除，或在存储分析
  点"删除所有已标记"并确认。
- 后续同步若视频重新出现在来源列表，标记会自动清掉。
- 仅先前出现在该来源待看列表的下载会被标记；搜索/播放列表等其他来源下载不受影响。
- 只在 provider 成功返回后做对比。认证、网络或解析失败时不推断远端缺失。

这里的"已看"是产品上的简称：实际信号只是"来源待看列表里已不存在"。B 站/YouTube
可能因用户手动移除、隐私/地区限制或条目失效而让视频消失，不能证明一定看完。因此先保留
文件，交由用户逐条或批量清理。

## 明确删除动作

| 操作 | 本地下载与文件 | 待看列表 |
|---|---|---|
| 离线库长按"标记看过并移除" | 删文件与下载记录 | 尝试写回来源；结果目前未在 App 展示 |
| 离线库长按"删除本地下载" | 只删本地文件与下载记录 | 保留 |
| 存储分析"删除所有已标记" | 批量删已标记文件与下载记录 | 保留远端已删除的状态 |

## B 站/YouTube 能力

| 目标 | 能力 | 依赖/限制 |
|---|---|---|
| 从来源待看列表移除 | B 站单条写回已实现（`POST /x/v2/history/toview/del`） | `SESSDATA`、`bili_jct`、`DedeUserID` |
| 标记已看但保留待看 | 未实现；B 站需 `POST /x/v2/history/report` 且要 aid/cid/位置 | provider 尚未实现 |
| YouTube 移除待看 | 未实现 | 需要 OAuth |
| 本地列表跟随远端删除 | 已实现：下一次成功同步就更新并移除本地待看成员 | 离线库下拉刷新、“已看”标签触发刷新，或设置页点“立即同步”；单独打开“源”页只读现有数据 |

B 站列表 API 返回的 `viewed` / `progress` 目前尚未落库；因此标记按"同步前后待看成员差异"
推断，而非使用 B 站自身 watched 标记。列表刷新必须成功，否则不清除/新增标记。

## 已知缺口

`markWatched` 会删除下载行，`autoEnqueue` 又通过下载行判断是否已入队；所以在远端移除写回失败
（如 YouTube 未支持或 cookie 过期）且 auto-download 开着时，后续 sync 可能把视频重新入队。
`remote_missing` 解决的是保留下载并供用户清理的流程；防止显式删除后自动复活仍需独立
的删除 tombstone。

App 目前未显示 `markWatched` 响应里的 `remote` 结果，远端写回失败时也没有重试队列。

## 刷新与下载的当前耦合

当前下拉刷新并不是"先取到新列表、逐条入库并让用户看到进度"的两段式流程：

1. App 先 `POST /api/sync`，该请求会同步等待 provider 读取远端列表；服务端在同一次请求里按 `auto_download` 决定是否为新条目排队下载。
2. 请求返回后 App 才重新读取 `GET /api/library`，而该接口只返回 `state='completed'` 且已有输出文件的下载，因此下载中的条目不会出现在离线库里。
3. 因此用户在刷新期间看到的是转圈，刷新结束后才看到已完成的项目；没有逐条加入列表、下载进度和"下载完成后可播放"的中间状态。
4. “源”页可以逐条下载：点击未下载的条目会 `POST /api/downloads?account_id=…`，之后 `reload()` 只重读该来源的待看与已下载集合，同样没有进度显示。

服务端已具备展示进度所需的数据：`GET /api/downloads` 返回每个任务的 `state`（`queued`/`downloading`/`paused`/`completed`/`failed`/`cancelled`）和 `progress`。要改成"先更新列表、再下载、可见进度"，需要在 App 侧把同步结果（待看列表）和下载任务分别渲染，并轮询或订阅下载状态；这不改变后端现有接口即可实现，但不属于当前行为。

## 代码入口

- App HTTP 客户端、`SyncParams`、来源聚合和请求/响应解析：NewPipe fork `app/src/main/java/org/schabi/newpipe/local/offline/VideodClient.kt`。
- 离线库的首次读取、下拉刷新和“已看”标签触发：`OfflineLibraryFragment.kt`。
- “源”页的只读加载与单条下载：`SourcesFragment.kt`。
- 设置读取、来源开关、立即同步和存储刷新：`SettingsFragment.kt`。
- 服务端 `/api/sync` 的来源选择、差异处理和自动入队：`backend/internal/api/handlers.go`、`sync.go`。
- 下载任务列表与进度字段：`backend/internal/media/media.go`。
- provider 注册：`backend/cmd/videod/main.go`；远端实现：`backend/internal/provider/youtube.go`、`bilibili.go`。

App 相关路径指向相邻的 NewPipe 仓库工作区，不包含在本仓库中。

## 验证

- `internal/api/sync_test.go: TestSyncDropsWatchLaterItemsRemovedUpstream`：验证列表删除、远端缺失标记、
  文件保留、视频回到远端后标记清除。
- 后端 `go test ./...` 全包通过；Android APK 已编译。

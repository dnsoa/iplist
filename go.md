# Go 使用说明（iplist）

本仓库提供两种使用方式：

1. **作为 Go module**：在你的服务中加载本项目生成的 `iplist.db`，实现 IP -> 国家/中国省市/运营商/云厂商查询。
2. **作为命令行工具（cmd）**：从仓库的 `data/` 目录构建 `iplist.db`，并进行查询或导出云厂商 IP 列表。

> 当前实现仅支持 **IPv4**。

---

## 1. 作为 Go module 使用

### 1.1 安装与引用

在你的项目中：

```bash
go get github.com/dnsoa/iplist
```

代码示例：

```go
package main

import (
	"fmt"

	"github.com/dnsoa/iplist"
)

func main() {
	db, err := iplist.Open("./iplist.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	res, ok, err := db.Lookup("1.2.3.4")
	if err != nil {
		panic(err)
	}
	if !ok {
		fmt.Println("no match")
		return
	}

	fmt.Println("country:", res.CountryCode, res.CountryName)
	fmt.Println("cn province:", res.CNProvinceCode, res.CNProvinceName)
	fmt.Println("cn city:", res.CNCityCode, res.CNCityName)
	fmt.Println("provider:", res.ProviderKey, res.ProviderName, res.ProviderKind)
}
```

### 1.2 API（当前可用）

- `iplist.Open(dbPath)`：打开由本仓库构建的数据库文件。
- `(*DB).Lookup(ip)`：查询单个 IP。
- `(*DB).CloudIPs(vendorKey)`：按云厂商 key 返回所有 CIDR（逐行字符串）。
- `(*DB).ProviderIPs(providerKey)`：按运营商/云厂商 key 返回所有 CIDR，并返回 `ProviderKind`。

`ProviderKind`：
- `ProviderKindISP`：运营商
- `ProviderKindCloud`：云厂商

---

## 2. 作为命令行工具使用（cmd/iplist）

### 2.1 构建数据库

在仓库根目录执行：

```bash
go run ./cmd/iplist build -data ./data -out ./iplist.db
```

也可以使用 `go generate`（类似 `golang.org/x/net/publicsuffix` 的生成器工作流）：

```bash
go generate ./...
```

说明：
- `-data` 指向仓库的 `data/` 目录（其下包含 `country/`、`cncity/`、`isp/`）。
- 构建会解析：
  - `data/country/*.txt`（国家，ISO3166-1 alpha-2）
  - `data/cncity/*.txt`（中国行政区划代码 6 位，省/市级）
  - `data/isp/*.txt`（运营商/云厂商，文件名作为 provider key）
- 国家/省市名称来自 `go generate ./...` 生成的紧凑名称表；若未生成或查不到则回退为 code/key。

### 2.2 查询 IP

```bash
go run ./cmd/iplist lookup -db ./iplist.db 1.2.3.4
```

输出字段包含：
- `country=CN (中国)`
- `cn_city=440300 (深圳市)` 或 `cn_province=440000 (广东省)`（取决于数据命中粒度）
- `provider=aliyun (阿里云) kind=2`

### 2.3 按云厂商导出所有 CIDR

例如导出阿里云：

```bash
go run ./cmd/iplist cloud -db ./iplist.db aliyun > aliyun.txt
```

### 2.4 按运营商/云厂商导出所有 CIDR

```bash
go run ./cmd/iplist provider -db ./iplist.db chinatelecom > chinatelecom.txt
```

### 2.5 导出 ID 对应表（便于导入外部数据库）

数据库内部查询热路径会返回 `ResultIDs`（例如 `CountryID` / `CNProvinceID` / `CNCityID` / `ProviderID`）。
你可以用 `export` 子命令把这些 ID 的含义导出为 TSV 表。

导出国家表：

```bash
go run ./cmd/iplist export -db ./iplist.db -what country > country.tsv
```

导出中国省/市表：

```bash
go run ./cmd/iplist export -db ./iplist.db -what cn_province > cn_province.tsv
go run ./cmd/iplist export -db ./iplist.db -what cn_city > cn_city.tsv
```

导出运营商/云厂商表：

```bash
go run ./cmd/iplist export -db ./iplist.db -what provider > provider.tsv
```

TSV 会带表头，列名分别为：
- `country_id, country_code, country_name`
- `cn_province_id, cn_province_code, cn_province_name`
- `cn_city_id, cn_city_code, cn_city_name`
- `provider_id, provider_key, provider_name, provider_kind`

也可以在 Go 代码里直接调用导出：
- `(*DB).ExportCountryTSV(w)`
- `(*DB).ExportCNProvinceTSV(w)`
- `(*DB).ExportCNCityTSV(w)`
- `(*DB).ExportProviderTSV(w)`

---

## 3. Provider key 列表

provider key 以 `data/isp/*.txt` 的文件名为准，例如：
- `aliyun` `tencent` `huawei` `microsoft` `googlecloud` `cloudflare` `digitalocean` `bytedance` `volcengine`
- `chinatelecom` `chinaunicom` `chinamobile` `drpeng` `cernet` `cstnet`

---

## 4. 已知限制

- 仅支持 IPv4。
- 构建阶段当前会校验同一类别内的区间不能出现“不同 label 的重叠”；如果未来数据源允许重叠（例如多标签归属），需要升级格式与查询逻辑。

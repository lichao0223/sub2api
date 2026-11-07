package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const AnalyticsSQLLimit = 20 << 20

func NormalizeAnalyticsName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\u200b' || r == '\ufeff' {
			return -1
		}
		return r
	}, norm.NFKC.String(name))
}

var analyticsInsert = regexp.MustCompile("(?is)^INSERT\\s+INTO\\s+`?([a-zA-Z_][a-zA-Z_0-9]*)`?\\s*(\\([^)]*\\))?\\s*VALUES\\s*")
var analyticsCreate = regexp.MustCompile("(?is)^CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([a-zA-Z_][a-zA-Z_0-9]*)`?\\s*\\(")
var analyticsColumn = regexp.MustCompile("^\\s*`([a-zA-Z_][a-zA-Z_0-9]*)`\\s+")
var analyticsLiteral = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)

var analyticsStandardColumns = map[string][]string{
	"devp_org":  strings.Split("id,meta_tenant_id,parent_id,level,path,org_code,org_name,org_desc,sort_no,status,create_by,create_time,update_by,update_time,del_flag,org_type", ","),
	"devp_user": strings.Split("id,user_code,username,password,real_name,email,phone,job_number,department,position,dev_org_id,status,work_status,off_duty_time,create_by,create_time,update_by,update_time,del_flag", ","),
}

// Split only outside quoted values/comments. No uploaded statement is executed.
func analyticsStatements(text string) ([]string, error) {
	var out []string
	var b strings.Builder
	quoted := byte(0)
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quoted != 0 {
			_ = b.WriteByte(c)
			if c == '\\' && quoted != '`' && i+1 < len(text) {
				i++
				_ = b.WriteByte(text[i])
			} else if c == quoted {
				if i+1 < len(text) && text[i+1] == quoted {
					i++
					_ = b.WriteByte(text[i])
				} else {
					quoted = 0
				}
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quoted = c
			_ = b.WriteByte(c)
		} else if c == '#' || (c == '-' && i+1 < len(text) && text[i+1] == '-') {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			_ = b.WriteByte('\n')
		} else if c == '/' && i+1 < len(text) && text[i+1] == '*' {
			end := strings.Index(text[i+2:], "*/")
			if end < 0 {
				return nil, errors.New("SQL注释未闭合")
			}
			i += end + 3
			_ = b.WriteByte(' ')
		} else if c == ';' {
			if s := strings.TrimSpace(b.String()); s != "" {
				out = append(out, s)
			}
			b.Reset()
		} else {
			_ = b.WriteByte(c)
		}
	}
	if quoted != 0 || strings.TrimSpace(b.String()) != "" {
		return nil, errors.New("SQL字符串未闭合或缺少分号")
	}
	return out, nil
}

func analyticsCreateColumns(definition string) ([]string, error) {
	var columns []string
	start, depth := 0, 0
	quoted := byte(0)
	for i := 0; i < len(definition); i++ {
		c := definition[i]
		if quoted != 0 {
			if c == '\\' && quoted != '`' && i+1 < len(definition) {
				i++
			} else if c == quoted {
				if i+1 < len(definition) && definition[i+1] == quoted {
					i++
				} else {
					quoted = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quoted = c
		case '(':
			depth++
		case ',', ')':
			if depth > 0 {
				if c == ')' {
					depth--
				}
				continue
			}
			// Only top-level definitions declare columns; index references and quoted metadata do not.
			if m := analyticsColumn.FindStringSubmatch(definition[start:i]); m != nil {
				columns = append(columns, m[1])
			}
			if c == ')' {
				return columns, nil
			}
			start = i + 1
		}
	}
	return nil, errors.New("CREATE TABLE括号未闭合")
}

func parseAnalyticsRows(text, expected string) ([]map[string]string, error) {
	if len(text) > AnalyticsSQLLimit || !utf8.ValidString(text) {
		return nil, errors.New("请上传20MiB以内的UTF-8 SQL文件")
	}
	statements, err := analyticsStatements(strings.TrimPrefix(text, "\ufeff"))
	if err != nil {
		return nil, err
	}
	columns := analyticsStandardColumns[expected]
	var rows []map[string]string
	seen := map[string]bool{}
	for _, statement := range statements {
		if m := analyticsCreate.FindStringSubmatchIndex(statement); m != nil {
			if statement[m[2]:m[3]] != expected {
				return nil, errors.New("SQL包含其他数据表")
			}
			declared, err := analyticsCreateColumns(statement[m[1]:])
			if err != nil {
				return nil, err
			}
			if len(declared) > 0 {
				columns = declared
			}
			continue
		}
		m := analyticsInsert.FindStringSubmatch(statement)
		if m == nil {
			upper := strings.ToUpper(strings.TrimSpace(statement))
			if upper == "SET FOREIGN_KEY_CHECKS = 0" || upper == "SET FOREIGN_KEY_CHECKS = 1" ||
				upper == "SET FOREIGN_KEY_CHECKS=0" || upper == "SET FOREIGN_KEY_CHECKS=1" ||
				upper == "SET NAMES UTF8MB4" || upper == "SET NAMES UTF8" || upper == "UNLOCK TABLES" ||
				upper == "LOCK TABLES `"+strings.ToUpper(expected)+"` WRITE" ||
				upper == "LOCK TABLES "+strings.ToUpper(expected)+" WRITE" ||
				upper == "DROP TABLE IF EXISTS `"+strings.ToUpper(expected)+"`" ||
				upper == "DROP TABLE IF EXISTS "+strings.ToUpper(expected) {
				continue
			}
			return nil, errors.New("只允许目标表的CREATE、字面值INSERT和常规导出设置")
		}
		if m[1] != expected {
			return nil, errors.New("INSERT表名不匹配")
		}
		keys := columns
		if m[2] != "" {
			keys = nil
			for _, key := range strings.Split(strings.Trim(m[2], "()"), ",") {
				keys = append(keys, strings.Trim(strings.TrimSpace(key), "`"))
			}
		}
		required := []string{"id", "real_name", "dev_org_id"}
		if expected == "devp_org" {
			required = []string{"id", "parent_id", "org_name"}
		}
		keySet := map[string]bool{}
		for _, key := range keys {
			if keySet[key] {
				return nil, fmt.Errorf("SQL列名重复：%s", key)
			}
			keySet[key] = true
		}
		for _, key := range required {
			if !keySet[key] {
				return nil, fmt.Errorf("缺少字段%s", key)
			}
		}
		data := statement[len(m[0]):]
		i := 0
		space := func() {
			for i < len(data) && (data[i] == ' ' || data[i] == '\r' || data[i] == '\n' || data[i] == '\t') {
				i++
			}
		}
		for i < len(data) {
			space()
			if i >= len(data) || data[i] != '(' {
				return nil, errors.New("INSERT VALUES格式错误")
			}
			i++
			var values []string
			for {
				space()
				var value strings.Builder
				if i < len(data) && data[i] == '\'' {
					i++
					closed := false
					for i < len(data) {
						c := data[i]
						i++
						if c == '\\' && i < len(data) {
							c = data[i]
							i++
							switch c {
							case 'n':
								c = '\n'
							case 'r':
								c = '\r'
							case 't':
								c = '\t'
							case '0':
								c = 0
							}
							_ = value.WriteByte(c)
						} else if c == '\'' {
							if i < len(data) && data[i] == '\'' {
								_ = value.WriteByte('\'')
								i++
							} else {
								closed = true
								break
							}
						} else {
							_ = value.WriteByte(c)
						}
					}
					if !closed {
						return nil, errors.New("SQL字符串未闭合")
					}
				} else {
					start := i
					for i < len(data) && data[i] != ',' && data[i] != ')' {
						i++
					}
					token := strings.TrimSpace(data[start:i])
					if !strings.EqualFold(token, "NULL") {
						if !analyticsLiteral.MatchString(token) {
							return nil, errors.New("SQL包含表达式，必须使用字面值导出")
						}
						_, _ = value.WriteString(token)
					}
				}
				values = append(values, value.String())
				space()
				if i >= len(data) || (data[i] != ',' && data[i] != ')') {
					return nil, errors.New("SQL行格式错误")
				}
				c := data[i]
				i++
				if c == ')' {
					break
				}
			}
			if len(values) != len(keys) {
				return nil, errors.New("SQL列和值数量不一致")
			}
			row := map[string]string{}
			for j, key := range keys {
				row[key] = values[j]
			}
			if row["id"] == "" || seen[row["id"]] {
				return nil, errors.New("存在空ID或重复ID")
			}
			seen[row["id"]] = true
			rows = append(rows, row)
			if len(rows) > 100000 {
				return nil, errors.New("每表最多100000行")
			}
			space()
			if i == len(data) {
				break
			}
			if data[i] != ',' {
				return nil, errors.New("不支持INSERT后附加语句")
			}
			i++
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("SQL中没有目录INSERT记录")
	}
	return rows, nil
}

func ParseAnalyticsDirectory(orgSQL, personSQL string) (AnalyticsDirectory, string, error) {
	var directory AnalyticsDirectory
	orgs, err := parseAnalyticsRows(orgSQL, "devp_org")
	if err != nil {
		return directory, "", fmt.Errorf("devp_org.sql：%w", err)
	}
	people, err := parseAnalyticsRows(personSQL, "devp_user")
	if err != nil {
		return directory, "", fmt.Errorf("devp_user.sql：%w", err)
	}
	index := map[string]int{}
	for _, row := range orgs {
		if row["del_flag"] == "1" {
			continue
		}
		name := strings.TrimSpace(row["org_name"])
		if NormalizeAnalyticsName(name) == "" || len(name) > 512 || len(row["id"]) > 128 {
			return directory, "", errors.New("组织名称或ID无效")
		}
		if row["id"] == "0" {
			return directory, "", errors.New("组织ID 0为待归属保留值")
		}
		parent := row["parent_id"]
		if parent == "" {
			parent = "0"
		}
		index[row["id"]] = len(directory.Orgs)
		directory.Orgs = append(directory.Orgs, AnalyticsOrg{ID: row["id"], Parent: parent, Name: name})
	}
	directoryBytes := 0
	for i := range directory.Orgs {
		o := &directory.Orgs[i]
		current := *o
		visited := map[string]bool{}
		for {
			if visited[current.ID] || len(o.Path) >= 16 {
				return directory, "", errors.New("组织存在循环或超过16级")
			}
			visited[current.ID] = true
			o.Path = append([]string{current.ID}, o.Path...)
			o.Names = append([]string{current.Name}, o.Names...)
			if current.Parent == "0" {
				break
			}
			parent, ok := index[current.Parent]
			if !ok {
				return directory, "", fmt.Errorf("组织%s缺少父节点", o.Name)
			}
			current = directory.Orgs[parent]
		}
		o.Level = len(o.Path)
		directoryBytes += len(strings.Join(o.Path, "/")) + len(strings.Join(o.Names, "/")) + 256
		if directoryBytes > AnalyticsSQLLimit {
			return directory, "", errors.New("展开后的组织目录超过20MiB，请精简目录")
		}
	}
	for _, row := range people {
		if row["del_flag"] == "1" {
			continue
		}
		name := strings.TrimSpace(row["real_name"])
		if NormalizeAnalyticsName(name) == "" || len(name) > 512 || len(row["id"]) > 128 {
			return directory, "", errors.New("人员姓名或ID无效")
		}
		if len(row["dev_org_id"]) > 128 {
			return directory, "", errors.New("人员组织ID过长")
		}
		active := (row["status"] == "" || row["status"] == strconv.Itoa(1)) && (row["work_status"] == "" || row["work_status"] == "1")
		directory.People = append(directory.People, AnalyticsPerson{ID: row["id"], Name: name, Org: row["dev_org_id"], Active: active})
	}
	if len(directory.Orgs) == 0 || len(directory.People) == 0 {
		return directory, "", errors.New("有效组织和人员不能为空")
	}
	data := []byte(AnalyticsJSON(directory))
	if len(data) > AnalyticsSQLLimit {
		return directory, "", errors.New("精简后目录超过20MiB")
	}
	sum := sha256.Sum256(data)
	return directory, hex.EncodeToString(sum[:]), nil
}

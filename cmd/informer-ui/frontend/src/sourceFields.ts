/**
 * Names of the subscription configuration columns, as the edit form writes them.
 *
 * The Go side declares these columns once, in internal/parsecfg, so that a
 * repair and a composed configuration cannot disagree about what they are
 * called. This is the same rule on this side: the diagnosis panel and the
 * composing chat both render a field list, and two copies of the mapping would
 * drift the moment a column is added.
 */
export const FIELD_LABELS: Record<string, string> = {
  url: '订阅 URL',
  curl: '自定义请求',
  parse_type: '解析类型',
  regex: '正则表达式',
  title_exp: '标题表达式',
  url_exp: '链接表达式',
  is_json: 'is_json 旧标志',
  json_title_path: 'JSON 标题路径',
  json_url_path: 'JSON 链接路径',
  agent_provider: 'Agent',
  agent_prompt: 'Agent 提示词',
  redirect: '链接重定向'
}

/** fieldLabel names a column, falling back to the raw name for an unknown one. */
export function fieldLabel(field: string): string {
  return FIELD_LABELS[field] ?? field
}

/**
 * fieldValue renders a configuration value for display, naming an empty one
 * rather than leaving a blank cell that reads like a rendering bug.
 */
export function fieldValue(value: string): string {
  return value === '' ? '（空）' : value
}

/** PARSE_TYPE_LABELS names the four parse types the way the edit form does. */
export const PARSE_TYPE_LABELS: Record<string, string> = {
  feed: '标准 feed（RSS / Atom）',
  regex: '正则匹配网页',
  json: 'JSON 接口',
  agent: '交给 AI Agent 去找'
}

/** parseTypeLabel names a parse type, falling back to the raw value. */
export function parseTypeLabel(parseType: string): string {
  return PARSE_TYPE_LABELS[parseType] ?? parseType
}

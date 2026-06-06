import type { Root } from 'mdast'
import type { Plugin } from 'unified'
import { visit } from 'unist-util-visit'

const HEADING_RE = /^(#{1,6})(?!\s)(?!\s*$)/
const FENCE_RE = /^\s*(```|~~~)/
const LOOSE_ATX_HEADING_RE = /^(\s{0,3})(#{1,6})(?=[^\s#])(.*)$/
const LOOSE_UNORDERED_LIST_RE = /^(\s{0,3})([-+*])(?=[^\s\-+*])/
const LOOSE_ORDERED_LIST_RE = /^(\s{0,3})(\d{1,9}[.)])(?=\S)/
const CJK_OR_ALNUM_BEFORE_STRONG_QUOTE_RE = /([\p{L}\p{N}])(\*\*(?=["'“‘（(][^*\n]+?\*\*))/gu
const STRONG_BEFORE_CJK_OR_ALNUM_RE = /(\*\*[^*\n]+?\*\*)(?=[\p{L}\p{N}])/gu

function normalizeInlineStrong(line: string): string {
  return line
    .replace(CJK_OR_ALNUM_BEFORE_STRONG_QUOTE_RE, '$1 $2')
    .replace(STRONG_BEFORE_CJK_OR_ALNUM_RE, '$1 ')
}

export function normalizeLooseMarkdown(input: string): string {
  let inFence = false

  return input
    .split('\n')
    .map((line) => {
      if (FENCE_RE.test(line)) {
        inFence = !inFence
        return line
      }
      if (inFence) return line

      return normalizeInlineStrong(line)
        .replace(LOOSE_ATX_HEADING_RE, '$1$2 $3')
        .replace(LOOSE_UNORDERED_LIST_RE, '$1$2 ')
        .replace(LOOSE_ORDERED_LIST_RE, '$1$2 ')
    })
    .join('\n')
}

const remarkFixHeadings: Plugin<[], Root> = () => {
  return (tree) => {
    visit(tree, 'paragraph', (node) => {
      if (node.children.length !== 1) return
      const child = node.children[0]
      if (child.type !== 'text') return

      const match = child.value.match(HEADING_RE)
      if (!match) return

      const level = match[1].length as 1 | 2 | 3 | 4 | 5 | 6
      const rest = child.value.slice(level)

      ;(node as any).type = 'heading'
      ;(node as any).depth = level
      ;(node as any).children = [{ type: 'text', value: rest }]
    })
  }
}

export default remarkFixHeadings

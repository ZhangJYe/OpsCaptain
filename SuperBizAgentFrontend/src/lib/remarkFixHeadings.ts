import type { Root } from 'mdast'
import type { Plugin } from 'unified'
import { visit } from 'unist-util-visit'

const HEADING_RE = /^(#{1,6})(?!\s)(?!\s*$)/

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

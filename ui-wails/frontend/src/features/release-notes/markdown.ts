import GithubSlugger from 'github-slugger';
import type { Element, Root, RootContent } from 'hast';
import type { Options } from 'rehype-sanitize';

export const maxNoteImages = 32;

export const noteSchema: Options = {
  tagNames: ['p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'em', 'strong', 'del',
    'blockquote', 'pre', 'code', 'hr', 'br', 'table', 'thead', 'tbody', 'tfoot', 'tr', 'th', 'td',
    'a', 'img', 'details', 'summary', 'span', 'div', 'sub', 'sup', 'kbd', 'input', 'section'],
  attributes: {
    '*': ['id', 'lang'],
    a: ['href', 'name', 'title', 'ariaLabel', 'ariaDescribedBy', 'dataFootnoteRef', 'dataFootnoteBackref'],
    img: ['src', 'alt', 'title'],
    ol: ['start'],
    th: ['align', 'colSpan', 'rowSpan'],
    td: ['align', 'colSpan', 'rowSpan'],
    code: [['className', /^language-[a-z0-9_-]+$/i]],
    h2: [['className', 'sr-only']],
    section: ['dataFootnotes', ['className', 'footnotes']],
    details: ['open'],
    input: [['type', 'checkbox'], 'checked', 'disabled'],
  },
  protocols: { href: ['http', 'https'], src: ['https'] },
  required: { input: { type: 'checkbox', disabled: true } },
  clobber: ['id', 'name', 'ariaDescribedBy'],
  clobberPrefix: 'release-note-',
  strip: ['script', 'style', 'iframe', 'object', 'embed', 'form', 'svg', 'math', 'textarea', 'select', 'button'],
};

function headingText(node: RootContent): string {
  if (node.type === 'text') return node.value;
  return node.type === 'element' ? node.children.map(headingText).join('') : '';
}

export function noteHeadings() {
  return (tree: Root) => {
    const slugger = new GithubSlugger();
    const visit = (node: Root | Element) => {
      for (const child of node.children) {
        if (child.type !== 'element') continue;
        if (child.tagName === 'a' && typeof child.properties.name === 'string' && !child.properties.id) {
          child.properties.id = child.properties.name;
        }
        if (/^h[1-6]$/.test(child.tagName) && !child.properties.id) {
          child.properties.id = slugger.slug(headingText(child));
        }
        if (/^(p|li|h[1-6])$/.test(child.tagName) && !child.properties.lang) {
          const text = headingText(child);
          const cjk = text.match(/[\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Han}]/gu)?.length ?? 0;
          const latin = text.match(/\p{Script=Latin}/gu)?.length ?? 0;
          if (cjk > latin) child.properties.lang = /[\p{Script=Hiragana}\p{Script=Katakana}]/u.test(text) ? 'ja' : 'zh';
        }
        visit(child);
      }
    };
    visit(tree);
  };
}

export function limitNoteImages() {
  return (tree: Root) => {
    let count = 0;
    const visit = (node: Root | Element) => {
      node.children = node.children.map(child => {
        if (child.type !== 'element') return child;
        if (child.tagName === 'img' && ++count > maxNoteImages) {
          const href = typeof child.properties.src === 'string' ? child.properties.src : '';
          const alt = typeof child.properties.alt === 'string' && child.properties.alt ? child.properties.alt : 'Image';
          return {
            type: 'element',
            tagName: 'a',
            properties: { href },
            children: [{ type: 'text', value: `${alt} omitted` }],
          };
        }
        visit(child);
        return child;
      });
    };
    visit(tree);
  };
}

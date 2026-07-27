import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type WorldMarkdownProps = {
  body: string;
};

export function WorldMarkdown({ body }: WorldMarkdownProps) {
  return (
    <div className="world-markdown">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ node: _node, children, ...props }) => (
            <a {...props} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
          table: ({ node: _node, children, ...props }) => (
            <div className="world-markdown__table-wrap">
              <table {...props}>{children}</table>
            </div>
          ),
          img: ({ node: _node, alt, src }) => (
            <span className="world-markdown__image">[图片：{alt || "未命名"}] {src}</span>
          ),
        }}
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}

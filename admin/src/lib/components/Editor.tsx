import MonacoEditor, { EditorProps } from "@monaco-editor/react";

const defaultMonacoOptions: EditorProps["options"] = {
  readOnly: true,
  wordWrap: "on",
  minimap: {
    enabled: false,
  },
};

type language = "html" | "typescript" | "json";

function languageForContentType(contentType?: string): language | undefined {
  const ct = contentType?.toLowerCase() ?? "";
  if (ct.startsWith("text/html")) return "html";
  if (ct.startsWith("application/json") || ct.includes("+json")) return "json";
  if (ct.startsWith("application/javascript")) return "typescript";
  return undefined;
}

interface Props {
  content: string;
  contentType?: string;
  monacoOptions?: EditorProps["options"];
  onChange?: EditorProps["onChange"];
}

function formatContent(content: string, contentType?: string): string {
  if (languageForContentType(contentType) === "json") {
    try {
      return JSON.stringify(JSON.parse(content), null, 2);
    } catch {
      // Not valid JSON — display as-is.
    }
  }
  return content;
}

function Editor({ content, contentType, monacoOptions, onChange }: Props): JSX.Element {
  return (
    <MonacoEditor
      language={languageForContentType(contentType)}
      theme="vs-dark"
      options={{ ...defaultMonacoOptions, ...monacoOptions }}
      value={formatContent(content, contentType)}
      onChange={onChange}
    />
  );
}

export default Editor;

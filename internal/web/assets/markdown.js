(function (global) {
  "use strict";

  const ALLOWED_TAGS = new Set([
    "p", "h1", "h2", "h3", "h4", "h5", "h6",
    "ul", "ol", "li",
    "blockquote", "pre", "code",
    "strong", "em", "del",
    "a", "hr", "br",
    "table", "thead", "tbody", "tr", "th", "td"
  ]);

  function escapeHTML(value) {
    return String(value || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function sanitizeHref(href) {
    const value = String(href || "").trim();
    if (!value) return "";

    if (/[\u0000-\u001f\u007f\s]/.test(value)) {
      return "";
    }

    const protocol = value.match(/^([a-zA-Z][a-zA-Z0-9+.-]*):/);
    if (protocol) {
      const allowed = protocol[1].toLowerCase();
      return allowed === "http" || allowed === "https" || allowed === "mailto" ? value : "";
    }

    if (value.startsWith("//")) return "";
    return value;
  }

  function parseInline(text) {
    let out = "";
    let i = 0;
    while (i < text.length) {
      if (text[i] === "`") {
        const end = text.indexOf("`", i + 1);
        if (end !== -1) {
          out += "<code>" + escapeHTML(text.slice(i + 1, end)) + "</code>";
          i = end + 1;
          continue;
        }
      }

      if (text[i] === "[") {
        const closeLabel = text.indexOf("]", i + 1);
        const openUrl = closeLabel !== -1 ? text.indexOf("(", closeLabel + 1) : -1;
        const closeUrl = openUrl !== -1 ? text.indexOf(")", openUrl + 1) : -1;
        if (closeLabel !== -1 && openUrl === closeLabel + 1 && closeUrl !== -1) {
          const label = text.slice(i + 1, closeLabel);
          const href = sanitizeHref(text.slice(openUrl + 1, closeUrl));
          if (href) {
            out += '<a href="' + escapeHTML(href) + '" rel="noopener noreferrer" target="_blank">'
              + parseInline(label) + "</a>";
          } else {
            out += escapeHTML(text.slice(i, closeUrl + 1));
          }
          i = closeUrl + 1;
          continue;
        }
      }

      if (text.startsWith("**", i)) {
        const end = text.indexOf("**", i + 2);
        if (end !== -1) {
          out += "<strong>" + parseInline(text.slice(i + 2, end)) + "</strong>";
          i = end + 2;
          continue;
        }
      }

      if (text.startsWith("__", i)) {
        const end = text.indexOf("__", i + 2);
        if (end !== -1) {
          out += "<strong>" + parseInline(text.slice(i + 2, end)) + "</strong>";
          i = end + 2;
          continue;
        }
      }

      if (text.startsWith("~~", i)) {
        const end = text.indexOf("~~", i + 2);
        if (end !== -1) {
          out += "<del>" + parseInline(text.slice(i + 2, end)) + "</del>";
          i = end + 2;
          continue;
        }
      }

      if (text[i] === "*" && text[i + 1] !== "*") {
        const end = text.indexOf("*", i + 1);
        if (end !== -1 && text[end + 1] !== "*") {
          out += "<em>" + parseInline(text.slice(i + 1, end)) + "</em>";
          i = end + 1;
          continue;
        }
      }

      if (text[i] === "_" && text[i + 1] !== "_") {
        const end = text.indexOf("_", i + 1);
        if (end !== -1 && text[end + 1] !== "_") {
          out += "<em>" + parseInline(text.slice(i + 1, end)) + "</em>";
          i = end + 1;
          continue;
        }
      }

      out += escapeHTML(text[i]);
      i += 1;
    }
    return out;
  }

  function isBlank(line) {
    return !String(line || "").trim();
  }

  function parseBlocks(source) {
    const lines = String(source || "").replace(/\r\n/g, "\n").split("\n");
    const blocks = [];
    let i = 0;

    while (i < lines.length) {
      const line = lines[i];

      if (isBlank(line)) {
        i += 1;
        continue;
      }

      if (line.startsWith("```")) {
        const fence = line.slice(3).trim();
        const body = [];
        i += 1;
        while (i < lines.length && !lines[i].startsWith("```")) {
          body.push(lines[i]);
          i += 1;
        }
        if (i < lines.length) i += 1;
        blocks.push({ type: "code", lang: fence, text: body.join("\n") });
        continue;
      }

      const heading = line.match(/^(#{1,6})\s+(.*)$/);
      if (heading) {
        blocks.push({ type: "heading", level: heading[1].length, text: heading[2] });
        i += 1;
        continue;
      }

      if (/^(-{3,}|\*{3,}|_{3,})$/.test(line.trim())) {
        blocks.push({ type: "hr" });
        i += 1;
        continue;
      }

      if (/^>\s?/.test(line)) {
        const quote = [];
        while (i < lines.length && /^>\s?/.test(lines[i])) {
          quote.push(lines[i].replace(/^>\s?/, ""));
          i += 1;
        }
        blocks.push({ type: "blockquote", text: quote.join("\n") });
        continue;
      }

      if (/^\s*[-*+]\s+/.test(line)) {
        const items = [];
        while (i < lines.length && /^\s*[-*+]\s+/.test(lines[i])) {
          items.push(lines[i].replace(/^\s*[-*+]\s+/, ""));
          i += 1;
        }
        blocks.push({ type: "ul", items });
        continue;
      }

      if (/^\s*\d+\.\s+/.test(line)) {
        const items = [];
        while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) {
          items.push(lines[i].replace(/^\s*\d+\.\s+/, ""));
          i += 1;
        }
        blocks.push({ type: "ol", items });
        continue;
      }

      const paragraph = [line];
      i += 1;
      while (i < lines.length && !isBlank(lines[i]) && !lines[i].startsWith("```")
        && !/^(#{1,6})\s+/.test(lines[i])
        && !/^>\s?/.test(lines[i])
        && !/^\s*[-*+]\s+/.test(lines[i])
        && !/^\s*\d+\.\s+/.test(lines[i])
        && !/^(-{3,}|\*{3,}|_{3,})$/.test(lines[i].trim())) {
        paragraph.push(lines[i]);
        i += 1;
      }
      blocks.push({ type: "p", text: paragraph.join("\n") });
    }

    return blocks;
  }

  function renderBlock(block) {
    switch (block.type) {
      case "code":
        return "<pre><code>" + escapeHTML(block.text) + "</code></pre>";
      case "heading":
        return "<h" + block.level + ">" + parseInline(block.text) + "</h" + block.level + ">";
      case "hr":
        return "<hr>";
      case "blockquote":
        return "<blockquote><p>" + parseInline(block.text).replace(/\n/g, "<br>") + "</p></blockquote>";
      case "ul":
        return "<ul>" + block.items.map(item => "<li>" + parseInline(item) + "</li>").join("") + "</ul>";
      case "ol":
        return "<ol>" + block.items.map(item => "<li>" + parseInline(item) + "</li>").join("") + "</ol>";
      case "p":
      default:
        return "<p>" + parseInline(block.text).replace(/\n/g, "<br>") + "</p>";
    }
  }

  function parseMarkdown(source) {
    return parseBlocks(source).map(renderBlock).join("");
  }

  function sanitizeNode(node) {
    const children = Array.from(node.childNodes);
    for (const child of children) {
      if (child.nodeType === Node.TEXT_NODE) {
        continue;
      }
      if (child.nodeType !== Node.ELEMENT_NODE) {
        child.remove();
        continue;
      }
      const tag = child.tagName.toLowerCase();
      if (!ALLOWED_TAGS.has(tag)) {
        while (child.firstChild) {
          node.insertBefore(child.firstChild, child);
        }
        child.remove();
        continue;
      }

      for (const attr of Array.from(child.attributes)) {
        if (tag === "a" && (attr.name === "href" || attr.name === "rel" || attr.name === "target")) {
          if (attr.name === "href") {
            const safe = sanitizeHref(attr.value);
            if (!safe) child.removeAttribute("href");
            else child.setAttribute("href", safe);
          }
          continue;
        }
        child.removeAttribute(attr.name);
      }

      if (tag === "a") {
        child.setAttribute("rel", "noopener noreferrer");
        if (/^https?:/i.test(child.getAttribute("href") || "")) {
          child.setAttribute("target", "_blank");
        }
      }

      sanitizeNode(child);
    }
  }

  function sanitizeHTML(html) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    sanitizeNode(doc.body);
    return doc.body.innerHTML;
  }

  function render(source) {
    const text = String(source || "").trim();
    if (!text) {
      return '<p class="markdown-empty muted">No content.</p>';
    }
    return sanitizeHTML(parseMarkdown(text));
  }

  global.tktMarkdown = { render };
})(window);

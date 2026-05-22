import json
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import fetch_ops_docs


class FetchOpsDocsTest(unittest.TestCase):
    def test_preprocess_removes_frontmatter_and_shortcodes(self):
        raw = """---
title: Demo
---
{{< note >}}
##
# Demo

<!-- hidden -->
content
"""
        got = fetch_ops_docs.preprocess(raw)
        self.assertNotIn("title: Demo", got)
        self.assertNotIn("{{<", got)
        self.assertNotIn("## \n", got)
        self.assertNotIn("hidden", got)
        self.assertIn("# Demo", got)
        self.assertIn("content", got)

    def test_write_doc_creates_metadata_sidecar(self):
        source = {
            "slug": "demo-doc",
            "provider": "kubernetes",
            "title": "Demo Doc",
            "url": "https://example.com/raw.md",
            "source_url": "https://example.com/doc.md",
            "license": "CC-BY-4.0",
            "tags": ["kubernetes", "debug"],
        }
        with tempfile.TemporaryDirectory() as tmp:
            md_path = fetch_ops_docs.write_doc(source, "# Body", pathlib.Path(tmp), "2026-05-22T00:00:00+00:00")
            self.assertTrue(md_path.exists())
            metadata_path = md_path.with_name("demo-doc.metadata.json")
            metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
            self.assertEqual(metadata["_source"], "upstream://kubernetes/demo-doc")
            self.assertEqual(metadata["license"], "CC-BY-4.0")
            self.assertEqual(metadata["tags"], ["kubernetes", "debug"])


if __name__ == "__main__":
    unittest.main()

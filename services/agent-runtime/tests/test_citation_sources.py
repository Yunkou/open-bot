"""Unit tests for citation/source URL collection."""

from __future__ import annotations

import json

from app.citation_sources import (
    extract_from_tool,
    filter_sources_cited,
    inject_citation_markers,
    merge_sources,
    normalize_url,
    sources_from_messages,
    structured_citation_urls,
    urls_from_answer_text,
)


def test_http_fetch_extracts_final_url() -> None:
    args = {"url": "https://example.com/a"}
    result = json.dumps(
        {
            "status": 200,
            "url": "https://example.com/a/",
            "body": "hello",
        }
    )
    got = extract_from_tool("http_fetch", args, result)
    assert len(got) == 1
    assert got[0]["url"].startswith("https://example.com/a")


def test_dedupe_and_ignore_non_http() -> None:
    a = merge_sources(
        [],
        [
            {"url": "https://a.example/x"},
            {"url": "https://a.example/x/"},
            {"url": "ftp://nope"},
            {"url": "not-a-url"},
        ],
    )
    assert len(a) == 1
    assert normalize_url(a[0]["url"]) == normalize_url("https://a.example/x")


def test_calculator_ignored() -> None:
    assert extract_from_tool("calculator", {"expression": "1+1"}, '{"result":2}') == []


def test_mcp_search_extracts_links() -> None:
    result = json.dumps(
        {
            "results": [
                {"title": "Docs", "url": "https://docs.example.com/guide"},
                {"title": "Blog", "link": "https://blog.example.com/post"},
            ]
        }
    )
    got = extract_from_tool("mcp__web__search", {"query": "x"}, result)
    urls = {s["url"] for s in got}
    assert "https://docs.example.com/guide" in urls
    assert "https://blog.example.com/post" in urls


def test_blocked_ssrf_skips() -> None:
    result = json.dumps({"error": "blocked address: 127.0.0.1", "url": "http://127.0.0.1/"})
    got = extract_from_tool("http_fetch", {"url": "http://127.0.0.1/"}, result)
    assert got == []


def test_sources_from_messages() -> None:
    msgs = [
        {
            "role": "assistant",
            "content": None,
            "tool_calls": [
                {
                    "id": "1",
                    "function": {
                        "name": "http_fetch",
                        "arguments": json.dumps({"url": "https://news.example/1"}),
                    },
                }
            ],
        },
        {
            "role": "tool",
            "tool_call_id": "1",
            "name": "http_fetch",
            "content": json.dumps({"status": 200, "url": "https://news.example/1", "body": "x"}),
        },
    ]
    got = sources_from_messages(msgs)
    assert len(got) == 1
    assert "news.example" in got[0]["url"]



def test_filter_sources_cited_keeps_only_urls_in_answer() -> None:
    sources = [
        {"url": "https://docs.example.com/guide", "title": "Docs"},
        {"url": "https://blog.example.com/post", "title": "Blog"},
        {"url": "https://noise.example.com/x"},
    ]
    answer = (
        "See the [Docs](https://docs.example.com/guide) for setup. "
        "Also https://blog.example.com/post has notes."
    )
    got = filter_sources_cited(sources, answer)
    urls = [s["url"] for s in got]
    assert urls == [
        "https://docs.example.com/guide",
        "https://blog.example.com/post",
    ]


def test_filter_sources_cited_soft_host_and_paraphrase_fallback() -> None:
    sources = [
        {"url": "https://docs.example.com/guide", "title": "Docs"},
        {"url": "https://blog.example.com/post"},
        {"url": "https://api.example.com/v1/x"},
        {"url": "https://news.example.com/rss.xml"},
    ]
    # Soft: answer names host without pasting a URL.
    soft = filter_sources_cited(sources, "I used docs.example.com for the steps.")
    assert [s["url"] for s in soft] == ["https://docs.example.com/guide"]
    # Paraphrase fallback: no cites, but retrieval clearly used — skip API/RSS noise.
    got = filter_sources_cited(sources, "I summarized the material without links.")
    urls = [s["url"] for s in got]
    assert "https://docs.example.com/guide" in urls
    assert "https://blog.example.com/post" in urls
    assert "https://api.example.com/v1/x" not in urls
    assert "https://news.example.com/rss.xml" not in urls
    # Empty / missing answer still yields nothing.
    assert filter_sources_cited(sources, "") == []
    assert filter_sources_cited(sources, None) == []


def test_filter_sources_cited_query_param_tolerance() -> None:
    sources = [{"url": "https://news.example.com/a?utm=1", "title": "News"}]
    answer = "Source: https://news.example.com/a"
    got = filter_sources_cited(sources, answer)
    assert len(got) == 1
    assert "news.example.com/a" in got[0]["url"]


def test_filter_sources_cited_structured_citations() -> None:
    sources = [
        {"url": "https://a.example/1"},
        {"url": "https://b.example/2"},
        {"url": "https://c.example/3"},
    ]
    # Answer cites none inline; structured citations name one.
    got = filter_sources_cited(
        sources,
        "Based on retrieval.",
        structured_citations=[{"url": "https://b.example/2", "title": "B"}],
    )
    assert [s["url"] for s in got] == ["https://b.example/2"]
    assert structured_citation_urls(["https://x.example/y"]) == ["https://x.example/y"]
    assert "docs.example.com" in urls_from_answer_text("see https://docs.example.com/a")[0]


def test_filter_preserves_order_of_sources_not_answer() -> None:
    sources = [
        {"url": "https://z.example/z"},
        {"url": "https://a.example/a"},
    ]
    answer = "first https://a.example/a then https://z.example/z"
    got = filter_sources_cited(sources, answer)
    assert [s["url"] for s in got] == ["https://z.example/z", "https://a.example/a"]


def test_html_fetch_does_not_scrape_body_hrefs() -> None:
    body = "<!DOCTYPE html><html><body>" + "".join(
        f'<a href="https://cdn{i}.microsoft.com/x">x</a>' for i in range(25)
    ) + "</body></html>"
    got = extract_from_tool(
        "http_fetch",
        {"url": "https://www.uino.com/about"},
        {
            "status": 200,
            "url": "https://www.uino.com/about",
            "content_type": "text/html",
            "body": body,
        },
    )
    assert len(got) == 1
    assert "uino.com" in got[0]["url"]


def test_merge_evicts_noisy_for_later_page() -> None:
    acc: list[dict[str, str]] = []
    # Fill with noisy SERP primary URLs
    for i in range(20):
        acc = merge_sources(
            acc,
            [{"url": f"https://cn.bing.com/search?q=x{i}"}],
        )
    assert len(acc) == 20
    acc = merge_sources(acc, [{"url": "https://www.uino.com/"}])
    urls = [s["url"] for s in acc]
    assert "https://www.uino.com/" in urls


def test_inject_citation_markers_soft_host() -> None:
    sources = [
        {"url": "https://www.uino.com/", "title": "UINO"},
        {"url": "https://www.thingjs.com/"},
    ]
    answer = "官网 uino.com 与 ThingJS 站。"
    got = inject_citation_markers(answer, sources)
    assert "[1]" in got and "[2]" in got
    assert inject_citation_markers(got, sources) == got


def test_filter_soft_host_label_thingjs() -> None:
    sources = [
        {"url": "https://cn.bing.com/search?q=x"},
        {"url": "https://www.thingjs.com/"},
        {"url": "https://www.uino.com/"},
    ]
    got = filter_sources_cited(sources, "优锘官网与 ThingJS 站已核对，见 uino.com")
    urls = [s["url"] for s in got]
    assert "https://www.uino.com/" in urls
    assert "https://www.thingjs.com/" in urls
    assert not any("bing.com" in u for u in urls)


if __name__ == "__main__":

    test_http_fetch_extracts_final_url()
    test_dedupe_and_ignore_non_http()
    test_calculator_ignored()
    test_mcp_search_extracts_links()
    test_blocked_ssrf_skips()
    test_sources_from_messages()
    test_filter_sources_cited_keeps_only_urls_in_answer()
    test_filter_sources_cited_soft_host_and_paraphrase_fallback()
    test_filter_sources_cited_query_param_tolerance()
    test_filter_sources_cited_structured_citations()
    test_filter_preserves_order_of_sources_not_answer()
    test_html_fetch_does_not_scrape_body_hrefs()
    test_merge_evicts_noisy_for_later_page()
    test_inject_citation_markers_soft_host()
    test_filter_soft_host_label_thingjs()
    print("ok")

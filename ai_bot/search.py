"""Поиск в интернете по явным запросам, без платного поискового API."""
import logging
from urllib.parse import urlsplit

LOG = logging.getLogger(__name__)


def search_sites(query, limit=4):
    try:
        from ddgs import DDGS
        found = DDGS(timeout=9).text(query[:220], max_results=limit)
        output = []
        seen = set()
        for item in found:
            url = str(item.get("href") or item.get("url") or "")
            host = urlsplit(url).hostname
            if not host or urlsplit(url).scheme not in ("http", "https") or host in seen:
                continue
            seen.add(host)
            output.append({"title": str(item.get("title", ""))[:180],
                           "body": str(item.get("body", ""))[:600],
                           "url": url[:500]})
            if len(output) >= limit:
                break
        return output
    except Exception as exc:
        LOG.warning("Поиск недоступен: %s", type(exc).__name__)
        return []


def format_results(results):
    return "\n".join(
        f"{i}. {r['title']}\n{r['body']}\nАдрес: {r['url']}"
        for i, r in enumerate(results, 1)
    )

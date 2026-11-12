import { computed, shallowRef } from 'vue'

import { listLinks } from '@/api'
import { ApiError } from '@/api/http'
import type { Link } from '@/api/types'

// useLinks 管理链接列表、游标分页和本地数据更新
export function useLinks(pageSize = 20) {
  const links = shallowRef<Link[]>([])
  const cursor = shallowRef('')
  const loading = shallowRef(false)
  const loadingMore = shallowRef(false)
  const errorMessage = shallowRef('')
  const hasMore = computed(() => Boolean(cursor.value))

  // load 重新加载第一页链接
  async function load(): Promise<void> {
    loading.value = true
    errorMessage.value = ''
    try {
      const result = await listLinks(pageSize)
      links.value = result.items
      cursor.value = result.next_cursor
    } catch (error) {
      errorMessage.value =
        error instanceof ApiError ? error.message : '加载短链接失败'
    } finally {
      loading.value = false
    }
  }

  // loadMore 使用游标追加下一页
  async function loadMore(): Promise<void> {
    if (!cursor.value || loadingMore.value) {
      return
    }
    loadingMore.value = true
    try {
      const result = await listLinks(pageSize, cursor.value)
      links.value = [...links.value, ...result.items]
      cursor.value = result.next_cursor
    } catch (error) {
      errorMessage.value =
        error instanceof ApiError ? error.message : '加载更多短链接失败'
    } finally {
      loadingMore.value = false
    }
  }

  // replaceLink 在编辑后替换列表中的对应链接
  function replaceLink(updated: Link): void {
    links.value = links.value.map((link) =>
      link.id === updated.id ? updated : link,
    )
  }

  // removeLink 在删除后移除本地行
  function removeLink(id: string): void {
    links.value = links.value.filter((link) => link.id !== id)
  }

  return {
    links,
    cursor,
    loading,
    loadingMore,
    errorMessage,
    hasMore,
    load,
    loadMore,
    replaceLink,
    removeLink,
  }
}

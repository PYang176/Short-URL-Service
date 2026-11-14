import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { guestOnly: true, title: '登录' },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/RegisterView.vue'),
      meta: { guestOnly: true, title: '注册' },
    },
    {
      path: '/try',
      name: 'try',
      component: () => import('@/views/TryView.vue'),
      meta: { guestOnly: true, title: '免登录试用' },
    },
    {
      path: '/report',
      name: 'report-abuse',
      component: () => import('@/views/ReportAbuseView.vue'),
      meta: { title: '举报违规短链' },
    },
    {
      path: '/',
      component: () => import('@/components/AppShell.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          redirect: () =>
            useAuthStore().isAuthenticated ? { name: 'dashboard' } : { name: 'try' },
        },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
          meta: { requiresAuth: true, title: '工作台' },
        },
        {
          path: 'links',
          name: 'links',
          component: () => import('@/views/LinksView.vue'),
          meta: { requiresAuth: true, title: '短链接' },
        },
        {
          path: 'links/:id',
          name: 'link-detail',
          component: () => import('@/views/LinkDetailView.vue'),
          meta: { requiresAuth: true, title: '链接详情' },
        },
        {
          path: 'domains',
          name: 'domains',
          component: () => import('@/views/DomainsView.vue'),
          meta: { requiresAuth: true, requiresAdmin: true, title: '自定义域名' },
        },
        {
          path: 'developer',
          name: 'developer',
          component: () => import('@/views/DeveloperView.vue'),
          meta: { requiresAuth: true, requiresAdmin: true, title: '开发者' },
        },
        {
          path: 'admin/abuse',
          name: 'abuse-admin',
          component: () => import('@/views/AbuseAdminView.vue'),
          meta: { requiresAuth: true, requiresAdmin: true, title: '滥用审核' },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
      meta: { title: '页面不存在' },
    },
  ],
  scrollBehavior(_to, _from, savedPosition) {
    return savedPosition ?? { top: 0 }
  },
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && auth.isAuthenticated) {
    return { name: 'dashboard' }
  }
  if (to.meta.requiresAdmin && auth.user?.role !== 2) {
    return { name: 'dashboard' }
  }
  return true
})

router.afterEach((to) => {
  const title = typeof to.meta.title === 'string' ? to.meta.title : ''
  document.title = title ? `${title} · 短链控制台` : '短链控制台'
})

export default router

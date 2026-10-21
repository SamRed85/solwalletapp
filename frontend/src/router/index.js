import { createRouter, createWebHistory } from 'vue-router'
import { auth, authReady } from '@/stores/auth'
import Home from '@/views/Home.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'Home', component: Home },
    {
      path: '/admin',
      name: 'Admin',
      component: () => import('@/views/Admin.vue'),
      meta: { requiresAdmin: true }
    },
    {
      path: '/users',
      name: 'Users',
      component: () => import('@/views/Users.vue'),
      meta: { requiresAdmin: true }
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

// UI guard only: the backend enforces admin access on every admin endpoint.
router.beforeEach(async to => {
  await authReady
  if (to.meta.requiresAdmin && !auth.isAdmin) return '/'
})

export default router

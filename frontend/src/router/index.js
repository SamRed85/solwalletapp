import { createRouter, createWebHistory } from 'vue-router'
import { auth, authReady } from '@/stores/auth'
import Home from '@/views/Home.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'Home', component: Home },
    {
      path: '/events/:id',
      name: 'Event',
      component: () => import('@/views/Event.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/profile',
      name: 'Profile',
      component: () => import('@/views/Profile.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/coupons',
      name: 'MyCoupons',
      component: () => import('@/views/MyCoupons.vue'),
      meta: { requiresAuth: true }
    },
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
  if (to.meta.requiresAuth && !auth.wallet) return '/'
})

export default router

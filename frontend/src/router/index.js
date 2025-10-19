import Vue from 'vue'
import VueRouter from 'vue-router'
import Home from '../views/Home.vue'
import Users from '../views/Users.vue'
import Admin from '../views/Admin.vue'
import Predictions from '../views/Predictions.vue'
import EventPredictions from '../views/EventPredictions.vue'
import MyCoupons from '../views/MyCoupons.vue'
import PaymentConfirmation from '../views/PaymentConfirmation.vue'

Vue.use(VueRouter)

const routes = [
  {
    path: '/',
    name: 'Home',
    component: Home
  },
  {
    path: '/users',
    name: 'Users',
    component: Users,
    meta: { requiresAuth: true, requiresAdmin: true }
  },
  {
    path: '/admin',
    name: 'Admin',
    component: Admin,
    meta: { requiresAuth: true, requiresAdmin: true }
  },
  {
    path: '/predictions',
    name: 'Predictions',
    component: Predictions,
    meta: { requiresAuth: true }
  },
  {
    path: '/predictions/:id',
    name: 'EventPredictions',
    component: EventPredictions,
    meta: { requiresAuth: true }
  },
  {
    path: '/my-coupons',
    name: 'MyCoupons',
    component: MyCoupons,
    meta: { requiresAuth: true }
  },
  {
    path: '/payment-confirmation',
    name: 'PaymentConfirmation',
    component: PaymentConfirmation,
    meta: { requiresAuth: true }
  }
]

const router = new VueRouter({
  mode: 'history',
  base: process.env.BASE_URL,
  routes
})

// Navigation guards for admin protection
router.beforeEach(async (to, from, next) => {
  // Check if route requires authentication
  if (to.matched.some(record => record.meta.requiresAuth)) {
    const isAuthenticated = localStorage.getItem('isAuthenticated') === 'true'
    
    if (!isAuthenticated) {
      next('/')
      return
    }
    
    // Check if route requires admin access
    if (to.matched.some(record => record.meta.requiresAdmin)) {
      try {
        const { adminService } = await import('@/services/api')
        const adminStatus = await adminService.checkAdminStatus()
        
        if (!adminStatus.isAdmin) {
          next('/')
          return
        }
      } catch (error) {
        console.error('Admin check failed:', error)
        next('/')
        return
      }
    }
  }
  
  next()
})

export default router

<template>
  <v-container class="py-6">
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon class="mr-2">mdi-account-group</v-icon>
        Users
      </v-card-title>
      <v-data-table :headers="headers" :items="users" :loading="loading" item-value="id">
        <template #[`item.walletAddress`]="{ value }">
          <code>{{ value }}</code>
        </template>
        <template #[`item.isAdmin`]="{ value }">
          <v-chip v-if="value" color="warning" size="small" variant="flat">admin</v-chip>
        </template>
        <template #[`item.createdAt`]="{ value }">{{ formatDate(value) }}</template>
        <template #[`item.lastLoginAt`]="{ value }">{{ formatDate(value) }}</template>
      </v-data-table>
    </v-card>
  </v-container>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { userApi } from '@/services/api'
import { notifyError } from '@/stores/notify'
import { formatDate } from '@/utils/format'

const headers = [
  { title: 'Wallet', key: 'walletAddress' },
  { title: 'Role', key: 'isAdmin' },
  { title: 'First sign-in', key: 'createdAt' },
  { title: 'Last sign-in', key: 'lastLoginAt' }
]
const users = ref([])
const loading = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    users.value = await userApi.list()
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
})
</script>

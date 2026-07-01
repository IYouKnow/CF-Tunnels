<template>
  <div class="skeleton" :style="containerStyle">
    <div v-for="i in count" :key="i" class="skeleton-item" :style="itemStyle" />
  </div>
</template>

<script>
export default {
  name: 'Skeleton',
  props: {
    width: { type: String, default: '100%' },
    height: { type: String, default: '1rem' },
    borderRadius: { type: String, default: '6px' },
    count: { type: Number, default: 1 }
  },
  computed: {
    containerStyle() {
      return {
        display: 'flex',
        flexDirection: 'column',
        gap: this.count > 1 ? '0.75rem' : '0'
      }
    },
    itemStyle() {
      return {
        width: this.width,
        height: this.height,
        borderRadius: this.borderRadius
      }
    }
  }
}
</script>

<style scoped>
.skeleton-item {
  background: linear-gradient(
    90deg,
    var(--bg-tertiary) 25%,
    var(--border) 50%,
    var(--bg-tertiary) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s ease-in-out infinite;
}

@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
</style>

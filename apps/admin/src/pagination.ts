/** Shared list-table pagination for admin main lists (antd Table / ProTable).
 * Use defaultPageSize (not pageSize) so the size changer is uncontrolled and sticks.
 */
export const LIST_PAGINATION = {
  defaultPageSize: 20,
  showSizeChanger: true,
  pageSizeOptions: ["10", "20", "50", "100"],
} as const;

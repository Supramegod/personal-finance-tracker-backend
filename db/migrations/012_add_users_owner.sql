-- Pisahkan "kolam user" dari keanggotaan kelompok.
--
-- Sebelumnya satu-satunya kaitan antara owner dan user yang dikelolanya
-- adalah lewat group_members -> groups.owner_user_id. Akibatnya user hanya
-- terlihat selama ia menjadi anggota suatu kelompok: begitu dikeluarkan dari
-- kelompok terakhirnya, akunnya tetap ada di tabel users tetapi hilang dari
-- daftar, tidak bisa di-drag, dan tidak bisa dikembalikan lewat UI.
--
-- Kolom ini menjadikan kepemilikan user sebagai fakta tersendiri, lepas dari
-- kelompok mana pun.
--
-- Nullable dengan sengaja: user yang mendaftar sendiri (bukan dibuat oleh
-- seorang owner) tidak dimiliki siapa pun, dan owner tingkat teratas juga
-- tidak punya induk.
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS owner_user_id UUID REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_users_owner ON users(owner_user_id);

-- Isi data lama: tetapkan pemilik dari kelompok tempat user itu sekarang
-- menjadi anggota, supaya tidak ada yang hilang dari daftar setelah
-- ListManagedUsers berpindah memakai kolom ini.
--
-- Satu user bisa menjadi anggota beberapa kelompok dari owner berbeda;
-- MIN(owner_user_id) memilih satu secara deterministik. Owner sebuah
-- kelompok tidak boleh menjadi pemilik dirinya sendiri.
UPDATE users u
SET owner_user_id = sub.owner_user_id
FROM (
  SELECT gm.user_id, MIN(g.owner_user_id::text)::uuid AS owner_user_id
  FROM group_members gm
  JOIN groups g ON g.id = gm.group_id
  WHERE g.owner_user_id <> gm.user_id
  GROUP BY gm.user_id
) AS sub
WHERE u.id = sub.user_id
  AND u.owner_user_id IS NULL;

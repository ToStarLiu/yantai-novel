ALTER TABLE wb_production_steps DROP CONSTRAINT wb_production_steps_chapter_id_fkey;
ALTER TABLE wb_production_steps ADD CONSTRAINT wb_production_steps_chapter_id_fkey
 FOREIGN KEY(chapter_id) REFERENCES wb_chapters(id) ON DELETE CASCADE;
